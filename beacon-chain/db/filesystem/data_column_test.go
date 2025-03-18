package filesystem

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	ckzg4844 "github.com/ethereum/c-kzg-4844/v2/bindings/go"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/spf13/afero"
)

type dataColumnParams struct {
	columnIndex uint64
	dataColumn  []byte // A whole data cell will be filled with the content of one item of this slice.
}

func dirLayout(layout string, fileName string) string {
	if layout == LayoutNameFlat {
		return fileName
	}

	return filepath.Join(periodicEpochBaseDir, "0", "0", fileName)
}

func createTestVerifiedRoDataColumnSidecars(t *testing.T, dataColumnParamsByBlockRoot map[[fieldparams.RootLength]byte][]dataColumnParams) []blocks.VerifiedRODataColumn {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	count := 0
	for _, indices := range dataColumnParamsByBlockRoot {
		count += len(indices)
	}

	verifiedRoDataColumnSidecars := make([]blocks.VerifiedRODataColumn, 0, count)
	for blockRoot, params := range dataColumnParamsByBlockRoot {
		for _, param := range params {
			dataColumn := make([][]byte, 0, len(param.dataColumn))
			for _, value := range param.dataColumn {
				cell := make([]byte, ckzg4844.BytesPerCell)
				for i := range ckzg4844.BytesPerCell {
					cell[i] = value
				}
				dataColumn = append(dataColumn, cell)
			}

			kzgCommitmentsInclusionProof := make([][]byte, 4)
			for i := range kzgCommitmentsInclusionProof {
				kzgCommitmentsInclusionProof[i] = make([]byte, 32)
			}

			dataColumnSidecar := &ethpb.DataColumnSidecar{
				ColumnIndex:                  param.columnIndex,
				DataColumn:                   dataColumn,
				KzgCommitmentsInclusionProof: kzgCommitmentsInclusionProof,
				SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
					Header: &ethpb.BeaconBlockHeader{
						ParentRoot: make([]byte, fieldparams.RootLength),
						StateRoot:  make([]byte, fieldparams.RootLength),
						BodyRoot:   make([]byte, fieldparams.RootLength),
					},
					Signature: make([]byte, fieldparams.BLSSignatureLength),
				},
			}

			roDataColumnSidecar, err := blocks.NewRODataColumnWithRoot(dataColumnSidecar, blockRoot)
			require.NoError(t, err)

			verifiedRoDataColumnSidecar := blocks.NewVerifiedRODataColumn(roDataColumnSidecar)
			verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, verifiedRoDataColumnSidecar)
		}
	}

	return verifiedRoDataColumnSidecars
}

func TestMetadata(t *testing.T) {
	for _, layout := range LayoutNames {
		t.Run("wrong version", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
					},
				},
			)

			// Save data columns into a file.
			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.NoError(t, err)

			// Alter the version.
			const fileName = "0x0100000000000000000000000000000000000000000000000000000000000000"
			file, err := blobStorage.fs.OpenFile(dirLayout(layout, fileName), os.O_WRONLY, os.FileMode(0600))
			require.NoError(t, err)

			count, err := file.Write([]byte{42})
			require.NoError(t, err)
			require.Equal(t, 1, count)

			// Try to read the metadata.
			_, err = blobStorage.metadata(file)
			require.ErrorIs(t, err, ErrWrongVersion)

			err = file.Close()
			require.NoError(t, err)
		})

		t.Run("wrong file size", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
					},
				},
			)

			// Save data columns into a file.
			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.NoError(t, err)

			// Append an extra byte to the file.
			const fileName = "0x0100000000000000000000000000000000000000000000000000000000000000"
			file, err := blobStorage.fs.OpenFile(dirLayout(layout, fileName), os.O_APPEND, os.FileMode(0600))
			require.NoError(t, err)

			count, err := file.Write([]byte{42})
			require.NoError(t, err)
			require.Equal(t, 1, count)

			// Try to read the metadata.
			_, err = blobStorage.metadata(file)
			require.ErrorIs(t, err, ErrWrongFileSize)

			err = file.Close()
			require.NoError(t, err)
		})
	}
}

func TestSaveDataColumnsSidecars(t *testing.T) {
	var dirLayout = func(layout string, fileName string) string {
		if layout == LayoutNameFlat {
			return fileName
		}

		return filepath.Join(periodicEpochBaseDir, "0", "0", fileName)
	}

	for _, layout := range LayoutNames {
		t.Run("wrong numbers of columns", func(t *testing.T) {
			originalNumberOfColumns := params.BeaconConfig().NumberOfColumns
			defer func() {
				params.BeaconConfig().NumberOfColumns = originalNumberOfColumns
			}()

			params.BeaconConfig().NumberOfColumns = 0

			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(nil)
			require.ErrorIs(t, err, ErrWrongNumberOfColumns)
		})

		t.Run("one of the column index is too large", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{}: []dataColumnParams{
						{columnIndex: 12},
						{columnIndex: 1_000_000},
						{columnIndex: 48},
					},
				},
			)

			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.ErrorIs(t, err, ErrDataColumnIndexTooLarge)
		})

		t.Run("new file - no data columns to save", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{}: []dataColumnParams{},
				},
			)

			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.NoError(t, err)
		})

		t.Run("new file - different data column size", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
						{columnIndex: 11, dataColumn: []byte{1, 2, 3, 4}},
					},
				},
			)

			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.ErrorIs(t, err, ErrWrongSszEncodedDataColumnSidecarSize)
		})

		t.Run("existing file - wrong incoming SSZ encoded size", func(t *testing.T) {
			verifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
					},
				},
			)

			// Save data columns into a file.
			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.NoError(t, err)

			// Build a data column sidecar for the same block but with a different
			// column index and an different SSZ encoded size.
			verifiedRoDataColumnSidecars = createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 13, dataColumn: []byte{1, 2, 3, 4}},
					},
				},
			)

			// Try to rewrite the file.
			err = blobStorage.SaveDataColumnSidecars(verifiedRoDataColumnSidecars)
			require.ErrorIs(t, err, ErrWrongSszEncodedDataColumnSidecarSize)
		})

		t.Run("nominal", func(t *testing.T) {
			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))

			inputVerifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
						{columnIndex: 11, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}}, // OK if duplicate
						{columnIndex: 13, dataColumn: []byte{6, 7, 8}},
					},
					[fieldparams.RootLength]byte{2}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 13, dataColumn: []byte{6, 7, 8}},
					},
				},
			)

			err := blobStorage.SaveDataColumnSidecars(inputVerifiedRoDataColumnSidecars)
			require.NoError(t, err)

			inputVerifiedRoDataColumnSidecars = createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}}, // OK if duplicate
						{columnIndex: 15, dataColumn: []byte{2, 3, 4}},
						{columnIndex: 1, dataColumn: []byte{2, 3, 4}},
					},
					[fieldparams.RootLength]byte{3}: []dataColumnParams{
						{columnIndex: 6, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 2, dataColumn: []byte{6, 7, 8}},
					},
				},
			)

			err = blobStorage.SaveDataColumnSidecars(inputVerifiedRoDataColumnSidecars)
			require.NoError(t, err)

			type fixture struct {
				fileName         string
				blockRoot        [fieldparams.RootLength]byte
				expectedIndices  [mandatoryNumberOfColumns]byte
				dataColumnParams []dataColumnParams
			}

			fixtures := []fixture{
				{
					fileName:  "0x0100000000000000000000000000000000000000000000000000000000000000",
					blockRoot: [fieldparams.RootLength]byte{1},
					expectedIndices: [mandatoryNumberOfColumns]byte{
						0, limit + 4, 0, 0, 0, 0, 0, 0,
						0, 0, 0, limit + 1, limit, limit + 2, 0, limit + 3,
						// The rest is filled with zeroes.
					},
					dataColumnParams: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
						{columnIndex: 11, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 13, dataColumn: []byte{6, 7, 8}},
						{columnIndex: 15, dataColumn: []byte{2, 3, 4}},
						{columnIndex: 1, dataColumn: []byte{2, 3, 4}},
					},
				},
				{
					fileName:  "0x0200000000000000000000000000000000000000000000000000000000000000",
					blockRoot: [fieldparams.RootLength]byte{2},
					expectedIndices: [mandatoryNumberOfColumns]byte{
						0, 0, 0, 0, 0, 0, 0, 0,
						0, 0, 0, 0, limit, limit + 1, 0, 0,
						// The rest is filled with zeroes.
					},
					dataColumnParams: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 13, dataColumn: []byte{6, 7, 8}},
					},
				},
				{
					fileName:  "0x0300000000000000000000000000000000000000000000000000000000000000",
					blockRoot: [fieldparams.RootLength]byte{3},
					expectedIndices: [mandatoryNumberOfColumns]byte{
						0, 0, limit + 1, 0, 0, 0, limit, 0,
						// The rest is filled with zeroes.
					},
					dataColumnParams: []dataColumnParams{
						{columnIndex: 6, dataColumn: []byte{3, 4, 5}},
						{columnIndex: 2, dataColumn: []byte{6, 7, 8}},
					},
				},
			}

			for _, fixture := range fixtures {
				// Build expected data column sidecars.
				expectedDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
					t,
					map[[fieldparams.RootLength]byte][]dataColumnParams{fixture.blockRoot: fixture.dataColumnParams},
				)

				// Build expected bytes.
				firstSszEncodedDataColumnSidecar, err := expectedDataColumnSidecars[0].MarshalSSZ()
				require.NoError(t, err)

				dataColumnSidecarsCount := len(expectedDataColumnSidecars)
				sszEncodedDataColumnSidecarSize := len(firstSszEncodedDataColumnSidecar)

				sszEncodedDataColumnSidecars := make([]byte, 0, dataColumnSidecarsCount*sszEncodedDataColumnSidecarSize)
				sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, firstSszEncodedDataColumnSidecar...)
				for _, dataColumnSidecar := range expectedDataColumnSidecars[1:] {
					sszEncodedDataColumnSidecar, err := dataColumnSidecar.MarshalSSZ()
					require.NoError(t, err)
					sszEncodedDataColumnSidecars = append(sszEncodedDataColumnSidecars, sszEncodedDataColumnSidecar...)
				}

				var encodedSszEncodedDataColumnSidecarSize [encodedSszEncodedDataColumnSidecarSizeSize]byte
				binary.BigEndian.PutUint32(encodedSszEncodedDataColumnSidecarSize[:], uint32(sszEncodedDataColumnSidecarSize))

				expectedBytes := make([]byte, 0, headerSize+dataColumnSidecarsCount*sszEncodedDataColumnSidecarSize)
				expectedBytes = append(expectedBytes, []byte{0x01}...)
				expectedBytes = append(expectedBytes, encodedSszEncodedDataColumnSidecarSize[:]...)
				expectedBytes = append(expectedBytes, fixture.expectedIndices[:]...)
				expectedBytes = append(expectedBytes, sszEncodedDataColumnSidecars...)

				// Check the actual content of the file.
				actualBytes, err := afero.ReadFile(blobStorage.fs, dirLayout(layout, fixture.fileName))
				require.NoError(t, err)
				require.DeepSSZEqual(t, expectedBytes, actualBytes)

				// Check the summary.
				indices := map[uint64]bool{}
				for _, dataColumnParam := range fixture.dataColumnParams {
					indices[dataColumnParam.columnIndex] = true
				}

				summary := blobStorage.Summary(fixture.blockRoot)
				for index := range uint64(mandatoryNumberOfColumns) {
					require.Equal(t, indices[index], summary.HasDataColumnIndex(index))
				}

				err = blobStorage.Remove(fixture.blockRoot)
				require.NoError(t, err)

				summary = blobStorage.Summary(fixture.blockRoot)
				for index := range uint64(mandatoryNumberOfColumns) {
					require.Equal(t, false, summary.HasDataColumnIndex(index))
				}

				_, err = afero.ReadFile(blobStorage.fs, dirLayout(layout, fixture.fileName))
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestSaveDataColumnsNewFile(t *testing.T) {
	for _, layout := range LayoutNames {
		t.Run("empty", func(t *testing.T) {
			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.saveDataColumnSidecarsNewFile("", []blocks.VerifiedRODataColumn{})
			require.NoError(t, err)
		})
	}
}

func TestClear(t *testing.T) {
	for _, layout := range LayoutNames {
		inputVerifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
			t,
			map[[fieldparams.RootLength]byte][]dataColumnParams{
				[fieldparams.RootLength]byte{1}: []dataColumnParams{{columnIndex: 12, dataColumn: []byte{1, 2, 3}}},
				[fieldparams.RootLength]byte{2}: []dataColumnParams{{columnIndex: 13, dataColumn: []byte{6, 7, 8}}},
			},
		)

		_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
		err := blobStorage.SaveDataColumnSidecars(inputVerifiedRoDataColumnSidecars)
		require.NoError(t, err)

		fileNames := []string{
			"0x0100000000000000000000000000000000000000000000000000000000000000",
			"0x0200000000000000000000000000000000000000000000000000000000000000",
		}

		for _, fileName := range fileNames {
			_, err = afero.ReadFile(blobStorage.fs, dirLayout(layout, fileName))
			require.NoError(t, err)
		}

		err = blobStorage.Clear()
		require.NoError(t, err)

		// TODO: Uncomment this when cache is cleared on blobStorage.Clear().
		// (Issue already on develop.)
		// summary := blobStorage.Summary([fieldparams.RootLength]byte{1})
		// for index := range uint64(mandatoryNumberOfColumns) {
		// 	require.Equal(t, false, summary.HasDataColumnIndex(index))
		// }

		for _, fileName := range fileNames {
			_, err = afero.ReadFile(blobStorage.fs, dirLayout(layout, fileName))
			require.ErrorIs(t, err, os.ErrNotExist)
		}
	}
}

func TestGetDataColumnSidecars(t *testing.T) {
	t.Run("index too large", func(t *testing.T) {
		_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameFlat))
		_, err := blobStorage.GetDataColumnSidecars([fieldparams.RootLength]byte{1}, []uint64{1_000_000})
		require.ErrorIs(t, err, ErrDataColumnIndexTooLarge)
	})

	for _, layout := range LayoutNames {
		t.Run("nominal", func(t *testing.T) {
			expectedVerifiedRoDataColumnSidecars := createTestVerifiedRoDataColumnSidecars(
				t,
				map[[fieldparams.RootLength]byte][]dataColumnParams{
					[fieldparams.RootLength]byte{1}: []dataColumnParams{
						{columnIndex: 12, dataColumn: []byte{1, 2, 3}},
						{columnIndex: 14, dataColumn: []byte{2, 3, 4}},
					},
				},
			)

			_, blobStorage := NewEphemeralBlobStorageAndFs(t, WithLayout(layout))
			err := blobStorage.SaveDataColumnSidecars(expectedVerifiedRoDataColumnSidecars)
			require.NoError(t, err)

			verifiedRODataColumnSidecars, err := blobStorage.GetDataColumnSidecars([fieldparams.RootLength]byte{1}, nil)
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedVerifiedRoDataColumnSidecars, verifiedRODataColumnSidecars)

			verifiedRODataColumnSidecars, err = blobStorage.GetDataColumnSidecars([fieldparams.RootLength]byte{1}, []uint64{12, 13, 14})
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedVerifiedRoDataColumnSidecars, verifiedRODataColumnSidecars)
		})
	}
}

// func TestBlobStorage_DataColumn_WithMigrationFromFlatToByEpoch(t *testing.T) {
// 	sidecars := setupDataColumnTest(t)

// 	// Setup flat layout
// 	fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameFlat))
// 	sidecar := sidecars[0]
// 	columnPath := bs.layout.sszPath(identForDataColumnSidecar(sidecar))
// 	data, err := ssz.MarshalSSZ(sidecar)
// 	require.NoError(t, err)
// 	require.NoError(t, bs.SaveDataColumnSidecars(sidecar))
// 	content, err := afero.ReadFile(fs, columnPath)
// 	require.NoError(t, err)
// 	require.Equal(t, true, bytes.Equal(data, content))

// 	// Setup by-epoch layout
// 	bs = NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))

// 	// Verify data is the same
// 	columnPath = bs.layout.sszPath(identForDataColumnSidecar(sidecar))
// 	content, err = afero.ReadFile(fs, columnPath)
// 	require.NoError(t, err)
// 	require.Equal(t, true, bytes.Equal(data, content))
// }

// func TestBlobStorage_DataColumn_WithMigrationFromByEpochToFlat(t *testing.T) {
// 	sidecars := setupDataColumnTest(t)

// 	// Setup by-epoch layout
// 	fs, bs := NewEphemeralBlobStorageAndFs(t, WithLayout(LayoutNameFlat))
// 	for _, sidecar := range sidecars {
// 		require.NoError(t, bs.SaveDataColumnSidecars(sidecar))
// 	}
// 	columnPath := bs.layout.sszPath(identForDataColumnSidecar(sidecars[0]))
// 	content, err := afero.ReadFile(fs, columnPath)
// 	require.NoError(t, err)
// 	data, err := ssz.MarshalSSZ(sidecars[0])
// 	require.NoError(t, err)
// 	require.Equal(t, true, bytes.Equal(data, content))

// 	// Setup flat layout
// 	bs = NewWarmedEphemeralBlobStorageUsingFs(t, fs, WithLayout(LayoutNameByEpoch))

// 	// Verify data is the same
// 	columnPath = bs.layout.sszPath(identForDataColumnSidecar(sidecars[0]))
// 	content, err = afero.ReadFile(fs, columnPath)
// 	require.NoError(t, err)
// 	require.Equal(t, true, bytes.Equal(data, content))
// }

// func setupDataColumnTest(t *testing.T) []blocks.VerifiedRODataColumn {
// 	// load trusted setup
// 	err := kzg.Start()
// 	require.NoError(t, err)

// 	// Setup right fork epoch
// 	params.SetupTestConfigCleanup(t)
// 	cfg := params.BeaconConfig().Copy()
// 	cfg.CapellaForkEpoch = 0
// 	cfg.DenebForkEpoch = 0
// 	cfg.ElectraForkEpoch = 0
// 	cfg.FuluForkEpoch = 0
// 	params.OverrideBeaconConfig(cfg)

// 	_, scs := util.GenerateTestFuluBlockWithSidecar(t, [32]byte{}, 0, 1)
// 	return verification.FakeVerifyDataColumnSliceForTest(t, scs)
// }
