package filesystem

import (
	"encoding/binary"
	"io"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/spf13/afero"
)

const (
	version                                    int = 0x01
	versionSize                                    = 1           //bytes
	maxSszEncodedDataColumnSidecarSize             = 536_870_912 // 2**(4*8) / 8 bytes
	encodedSszEncodedDataColumnSidecarSizeSize     = 4           // bytes (size of the endoded size of the SSZ encoded data column sidecar)
	mandatoryNumberOfColumns                       = 128         // 2**7
	limit                                          = mandatoryNumberOfColumns
	headerSize                                     = versionSize + encodedSszEncodedDataColumnSidecarSizeSize + mandatoryNumberOfColumns
)

var (
	ErrEncodedDataColumnSidecarTooLarge     = errors.New("encoded data columns sidecar too large")
	ErrWrongNumberOfColumns                 = errors.New("wrong number of data columns")
	ErrDataColumnIndexTooLarge              = errors.New("data column index too large")
	ErrWrongBytesWritten                    = errors.New("wrong number of bytes written")
	ErrWrongBytesVersionRead                = errors.New("wrong number of bytes version read")
	ErrWrongVersion                         = errors.New("wrong version")
	ErrWrongBytesDataColumnSidecarSizeRead  = errors.New("wrong number of bytes data column sidecar size read")
	ErrWrongBytesIndicesRead                = errors.New("wrong number of bytes indices read")
	ErrWrongFileSize                        = errors.New("wrong file size")
	ErrTooManyDataColumns                   = errors.New("too many data columns")
	ErrWrongSszEncodedDataColumnSidecarSize = errors.New("wrong SSZ encoded data column sidecar size")
)

type metadata struct {
	indices                         [mandatoryNumberOfColumns]byte
	savedDataColumnSidecarCount     uint32
	sszEncodedDataColumnSidecarSize uint32
	fileSize                        int64
}

// SaveDataColumnSidecars saves data column sidecars into the database.
func (bs *BlobStorage) SaveDataColumnSidecars(dataColumnSidecars []blocks.VerifiedRODataColumn) error {
	// Check the number of columns is the one expected.
	// While implementing this, we expect the number of columns won't change.
	// If it does, we will need to create a new version of the data column sidecar file.
	if params.BeaconConfig().NumberOfColumns != mandatoryNumberOfColumns {
		return ErrWrongNumberOfColumns
	}

	dataColumnSidecarsbyDir := make(map[string][]blocks.VerifiedRODataColumn)
	idents := make([]blobIdent, len(dataColumnSidecars))

	// Group data column sidecars by dir.
	for _, dataColumnSidecar := range dataColumnSidecars {
		// Check if the data column index is too large.
		if dataColumnSidecar.ColumnIndex >= mandatoryNumberOfColumns {
			return ErrDataColumnIndexTooLarge
		}

		// Compute the ident for the data column sidecar.
		ident := identForDataColumnSidecar(dataColumnSidecar)

		// Group data column sidecars by dir.
		dir := bs.layout.dir(ident)
		dataColumnSidecarsbyDir[dir] = append(dataColumnSidecarsbyDir[dir], dataColumnSidecar)

		// Save the ident for later use.
		idents = append(idents, ident)
	}

	// Save the data column sidecars.
	for dir, dataColumnSidecars := range dataColumnSidecarsbyDir {
		exists, err := afero.Exists(bs.fs, dir)
		if err != nil {
			return errors.Wrap(err, "afero exists")
		}

		if exists {
			if err := bs.saveDataColumnSidecarsExistingFile(dir, dataColumnSidecars); err != nil {
				return errors.Wrap(err, "save data column existing file")
			}

			continue
		}

		if err := bs.saveDataColumnSidecarsNewFile(dir, dataColumnSidecars); err != nil {
			return errors.Wrap(err, "save data columns new file")
		}
	}

	// Notify the data column notifier that a new data column has been saved.
	for _, ident := range idents {
		if err := bs.layout.notify(ident); err != nil {
			return errors.Wrapf(err, "problem maintaining pruning cache/metrics for sidecar with root=%#x", ident.root)
		}

		if bs.DataColumnFeed != nil {
			rootPairIndex := RootIndexPair{Root: ident.root, Index: ident.index}
			bs.DataColumnFeed.Send(rootPairIndex)
		}

		blobsWrittenCounter.Inc()
	}

	return nil
}

// GetDataColumnSidecars retrieves data column sidecars from the database.
// If one of the requested data column sidecars is not found, it is just skipped.
// If indices is nil, then all stored data column sidecars are returned.
// Since BlobStorage only writes data columns that have undergone full verification, the return
// value is always a VerifiedRODataColumn.
func (bs *BlobStorage) GetDataColumnSidecars(root [fieldparams.RootLength]byte, indices []uint64) ([]blocks.VerifiedRODataColumn, error) {
	startTime := time.Now()

	// Build all indices if none are provided.
	if indices == nil {
		indices = make([]uint64, mandatoryNumberOfColumns)
		for i := range indices {
			indices[i] = uint64(i)
		}
	}

	// Preventive check of indices.
	for _, index := range indices {
		if index >= mandatoryNumberOfColumns {
			return nil, ErrDataColumnIndexTooLarge
		}
	}

	// Compute the file path.
	var filePath string
	for _, index := range indices {
		ident, err := bs.layout.ident(root, index)
		if err == db.ErrNotFound {
			continue
		}

		if err != nil {
			return nil, errors.Wrap(err, "ident")
		}

		localDir := bs.layout.dir(ident)

		if len(filePath) == 0 {
			filePath = localDir
			continue
		}

		if filePath != localDir {
			return nil, errors.New("data column sidecars with the same root from different directories (should never happen)")
		}
	}

	// Open the data column sidecars file.
	file, err := bs.fs.OpenFile(filePath, os.O_RDONLY, os.FileMode(0400))
	if err != nil {
		return nil, errors.Wrap(err, "data column sidecars file path open")
	}

	// Read file metadata.
	metadata, err := bs.metadata(file)
	if err != nil {
		return nil, errors.Wrap(err, "metadata")
	}

	// Retrieve data column sidecars from the file.
	verifiedRODataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, len(indices))
	for _, index := range indices {
		// Skip if the data column is not saved.
		if metadata.indices[index] < limit {
			continue
		}

		// Compute the offset of the data column sidecar.
		offset := headerSize + int64(metadata.indices[index]-limit)*int64(metadata.sszEncodedDataColumnSidecarSize)

		// Seek to the beginning of the data column sidecar.
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, errors.Wrap(err, "seek")
		}

		// Read the SSZ encoded data column sidecar.
		sszEncodedDataColumnSidecar := make([]byte, metadata.sszEncodedDataColumnSidecarSize)
		count, err := file.Read(sszEncodedDataColumnSidecar)
		if err != nil {
			return nil, errors.Wrap(err, "read SSZ encoded data column sidecar")
		}
		if uint32(count) != metadata.sszEncodedDataColumnSidecarSize {
			return nil, ErrWrongBytesWritten
		}

		// Unmarshal the SSZ encoded data column sidecar.
		dataColumnSidecar := &ethpb.DataColumnSidecar{}
		err = dataColumnSidecar.UnmarshalSSZ(sszEncodedDataColumnSidecar)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal SSZ encoded data column sidecar")
		}

		// Create a RO data column.
		roDataColumnSidecar, err := blocks.NewRODataColumn(dataColumnSidecar)
		if err != nil {
			return nil, errors.Wrap(err, "new read only data column")
		}

		// Create a verified RO data column.
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)

		// Append the verified RO data column to the data column sidecars.
		verifiedRODataColumnSidecars = append(verifiedRODataColumnSidecars, verifiedRODataColumn)
	}

	blobFetchLatency.Observe(float64(time.Since(startTime).Milliseconds()))

	return verifiedRODataColumnSidecars, nil
}

// saveDataColumnSidecarsExistingFile saves data column sidecars into an existing file.
// This function expects all data column sidecars to belong to the same block.
func (bs *BlobStorage) saveDataColumnSidecarsExistingFile(filePath string, dataColumnSidecars []blocks.VerifiedRODataColumn) (err error) {
	// Compute the count of data column sidecars.
	dataColumnSidecarsCount := uint32(len(dataColumnSidecars))

	// Open the data column sidecars file.
	file, err := bs.fs.OpenFile(filePath, os.O_RDWR, os.FileMode(0600))
	if err != nil {
		return errors.Wrap(err, "data column sidecars file path open")
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	metadata, err := bs.metadata(file)
	if err != nil {
		return errors.Wrap(err, "metadata")
	}

	// Create the SSZ encoded data column sidecars.
	sszEncodedDataColumnSidecars := make([]byte, 0, dataColumnSidecarsCount*metadata.sszEncodedDataColumnSidecarSize)

	for _, dataColumnSidecar := range dataColumnSidecars {
		// Extract the data columns index.
		dataColumnIndex := dataColumnSidecar.ColumnIndex

		// Skip if the data column is already saved.
		if metadata.indices[dataColumnIndex] >= limit {
			continue
		}

		// Check if the number of saved data columns is too large.
		// This is impossible to happen in practice is this function is called
		// by SaveDataColumnSidecars.
		if metadata.savedDataColumnSidecarCount >= mandatoryNumberOfColumns {
			return ErrTooManyDataColumns
		}

		// SSZ encode the data column sidecar.
		sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "data column sidecar marshal SSZ")
		}

		// Compute the size of the SSZ encoded data column sidecar.
		incomingSszEncodedDataColumnSidecarSize := uint32(len(sszEncodedDataColumnSidecar))

		// Check if the incoming encoded data column sidecar size corresponds to the one read from the file.
		if incomingSszEncodedDataColumnSidecarSize != metadata.sszEncodedDataColumnSidecarSize {
			return ErrWrongSszEncodedDataColumnSidecarSize
		}

		// Alter indices to mark the data column as saved.
		// savedDataColumnsCount can safely be cast to uint8 since we have checked it is less than mandatoryNumberOfColumns.
		metadata.indices[dataColumnIndex] = limit + uint8(metadata.savedDataColumnSidecarCount)
		metadata.savedDataColumnSidecarCount++

		// Append the SSZ encoded data column sidecar to the SSZ encoded data column sidecars.
		sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
	}

	// Save indices to the file.
	count, err := file.WriteAt(metadata.indices[:], int64(versionSize+encodedSszEncodedDataColumnSidecarSizeSize))
	if err != nil {
		return errors.Wrap(err, "write indices")
	}
	if count != mandatoryNumberOfColumns {
		return ErrWrongBytesWritten
	}

	// Append the SSZ encoded data column sidecars to the end of the file.
	count, err = file.WriteAt(sszEncodedDataColumnSidecars, metadata.fileSize)
	if err != nil {
		return errors.Wrap(err, "write SSZ encoded data column sidecars")
	}
	if count != len(sszEncodedDataColumnSidecars) {
		return ErrWrongBytesWritten
	}

	return nil
}

// saveDataColumnSidecarsNewFile saves data column sidecars into a new file.
// This function expects all data column sidecars to belong to the same block.
func (bs *BlobStorage) saveDataColumnSidecarsNewFile(filePath string, dataColumnSidecars []blocks.VerifiedRODataColumn) (err error) {
	// Get the count of data column sidecars.
	dataColumnSidecarsCount := len(dataColumnSidecars)

	// Exit early if there are no data column sidecars to save.
	if dataColumnSidecarsCount == 0 {
		return nil
	}

	// Initialize the indices.
	var indices [mandatoryNumberOfColumns]byte

	// Safely retrieve the first data column sidecar.
	firstDataColumnSidecar := dataColumnSidecars[0]

	// Extract the data column index.
	firstColumnIndex := firstDataColumnSidecar.ColumnIndex

	// Alter the indices to mark the first data column sidecar as saved.
	indices[firstColumnIndex] = limit + 0

	// SSZ encode the first data column sidecar.
	sszEncodedDataColumnSidecar, err := firstDataColumnSidecar.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "data column sidecar marshal SSZ")
	}

	// Compute the size of the SSZ encoded data column sidecar.
	sszEncodedDataColumnSidecarSize := len(sszEncodedDataColumnSidecar)

	// Encode the size of the SSZ encoded data column sidecar.
	var encodedSszEncodedDataColumnSidecarSize [encodedSszEncodedDataColumnSidecarSizeSize]byte
	binary.BigEndian.PutUint32(encodedSszEncodedDataColumnSidecarSize[:], uint32(sszEncodedDataColumnSidecarSize))

	// Create the SSZ encoded data column sidecars.
	sszEncodedDataColumnSidecars := make([]byte, 0, dataColumnSidecarsCount*sszEncodedDataColumnSidecarSize)

	// Append the first SSZ encoded data column sidecar to the SSZ encoded data column sidecars.
	sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)

	// Initialize the count of the saved SSZ encoded data column sidecar.
	savedCount := 1

	// SSZ encode all the remaining data column sidecars.
	for _, dataColumnSidecar := range dataColumnSidecars[1:] {
		// Extract the data column index.
		dataColumnIndex := dataColumnSidecar.ColumnIndex

		// Skip if the data column is already saved.
		if indices[dataColumnIndex] >= limit {
			continue
		}

		// Alter the indices to mark the first data column sidecar as saved.
		// savedCount can safely be cast to uint8 since it is less than limit.
		indices[dataColumnIndex] = limit + uint8(savedCount)

		// Increment the count of the saved SSZ encoded data column sidecar.
		savedCount++

		// SSZ encode the first data column sidecar.
		sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
		if err != nil {
			return errors.Wrap(err, "data column sidecar marshal SSZ")
		}

		// Check if the size of the SSZ encoded data column sidecar is correct.
		if len(sszEncodedDataColumnSidecar) != sszEncodedDataColumnSidecarSize {
			return ErrWrongSszEncodedDataColumnSidecarSize
		}

		// Append the SSZ encoded data column sidecar to the SSZ encoded data column sidecars.
		sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
	}

	// Concatenate the version, the data column sidecar size, the data column indices and the SSZ encoded data column sidecar.
	countToWrite := headerSize + savedCount*sszEncodedDataColumnSidecarSize
	bytes := make([]byte, 0, countToWrite)
	bytes = append(bytes, byte(version))
	bytes = append(bytes, encodedSszEncodedDataColumnSidecarSize[:]...)
	bytes = append(bytes, indices[:]...)
	bytes = append(bytes, sszEncodedDataColumnSidecars...)

	// Create the data column sidecars file.
	file, err := bs.fs.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "data column sidecars file path create")
	}

	defer func() {
		closeErr := file.Close()

		// Overwrite the existing error only if it is nil, since the close error is less important.
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	countWritten, err := file.Write(bytes)
	if err != nil {
		return errors.Wrap(err, "write")
	}
	if countWritten != countToWrite {
		return ErrWrongBytesWritten
	}

	return nil
}

// metadata runs file sanity checks and retrieves metadata of the file.
// The file descriptor is left at the beginning of the first SSZ encoded data column sidecar.
func (bs *BlobStorage) metadata(file afero.File) (*metadata, error) {
	// Seek to the beginning of the file.
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, errors.Wrap(err, "seek")
	}

	// Read the encoded file version.
	var encodedFileVersion [versionSize]byte
	countRead, err := file.Read(encodedFileVersion[:])
	if err != nil {
		return nil, errors.Wrap(err, "read file version")
	}

	if countRead != versionSize {
		return nil, ErrWrongBytesVersionRead
	}

	// Convert the version to an int.
	fileVersion := int(encodedFileVersion[0])

	// Check if the version is the expected one.
	if fileVersion != version {
		return nil, ErrWrongVersion
	}

	// Read the SSZ encoded data column sidecar size.
	var encodedSszEncodedDataColumnSidecarSize [encodedSszEncodedDataColumnSidecarSizeSize]byte
	countRead, err = file.Read(encodedSszEncodedDataColumnSidecarSize[:])
	if err != nil {
		return nil, errors.Wrap(err, "read SSZ encoded data column sidecar size")
	}
	if countRead != encodedSszEncodedDataColumnSidecarSizeSize {
		return nil, ErrWrongBytesDataColumnSidecarSizeRead
	}

	// Convert the SSZ encoded data column sidecar size to an int.
	sszEncodedDataColumnSidecarSize := binary.BigEndian.Uint32(encodedSszEncodedDataColumnSidecarSize[:])

	// Read the data column indices.
	var indices [mandatoryNumberOfColumns]byte
	countRead, err = file.Read(indices[:])
	if err != nil {
		return nil, errors.Wrap(err, "read data column indices")
	}
	if countRead != mandatoryNumberOfColumns {
		return nil, ErrWrongBytesIndicesRead
	}

	// Retrieve the statistics of the file.
	fileStat, err := file.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "file stat")
	}

	// Get the size of the file.
	fileSize := fileStat.Size()

	// Check the file size is correct.
	if uint32(fileSize-headerSize)%sszEncodedDataColumnSidecarSize != 0 {
		return nil, ErrWrongFileSize
	}

	// Compute how many data columns are saved.
	savedDataColumnsCount := uint32(fileSize-headerSize) / sszEncodedDataColumnSidecarSize

	metadata := &metadata{
		indices:                         indices,
		savedDataColumnSidecarCount:     savedDataColumnsCount,
		sszEncodedDataColumnSidecarSize: sszEncodedDataColumnSidecarSize,
		fileSize:                        fileSize,
	}

	return metadata, nil
}
