package types

import (
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
)

func init() {
	// Initialize data maps.
	InitializeDataMaps()
}

// This file provides a mapping of fork version to the respective data type. This is
// to allow any service to appropriately use the correct data type with a provided
// fork-version.

var (
	// BlockMap maps the fork-version to the underlying data type for that
	// particular fork period.
	BlockMap map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error)
	// MetaDataMap maps the fork-version to the underlying data type for that
	// particular fork period.
	MetaDataMap map[[4]byte]func() (metadata.Metadata, error)
	// AttestationMap maps the fork-version to the underlying data type for that
	// particular fork period.
	AttestationMap map[[4]byte]func() (ethpb.Att, error)
	// AggregateAttestationMap maps the fork-version to the underlying data type for that
	// particular fork period.
	AggregateAttestationMap map[[4]byte]func() (ethpb.SignedAggregateAttAndProof, error)

	LightClientOptimisticUpdateMap map[[4]byte]func() (interfaces.LightClientOptimisticUpdate, error)
	LightClientFinalityUpdateMap   map[[4]byte]func() (interfaces.LightClientFinalityUpdate, error)
)

// InitializeDataMaps initializes all the relevant object maps. This function is called to
// reset maps and reinitialize them.
func InitializeDataMaps() {
	// Reset our block map.
	BlockMap = map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{Body: &ethpb.BeaconBlockBodyAltair{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{Body: &ethpb.BeaconBlockBodyBellatrix{ExecutionPayload: &enginev1.ExecutionPayload{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockCapella{Block: &ethpb.BeaconBlockCapella{Body: &ethpb.BeaconBlockBodyCapella{ExecutionPayload: &enginev1.ExecutionPayloadCapella{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockDeneb{Block: &ethpb.BeaconBlockDeneb{Body: &ethpb.BeaconBlockBodyDeneb{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockFulu{Block: &ethpb.BeaconBlockFulu{Body: &ethpb.BeaconBlockBodyFulu{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
			)
		},
	}

	// Reset our metadata map.
	MetaDataMap = map[[4]byte]func() (metadata.Metadata, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV0(&ethpb.MetaDataV0{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
	}

	// Reset our attestation map.
	AttestationMap = map[[4]byte]func() (ethpb.Att, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (ethpb.Att, error) {
			return &ethpb.AttestationElectra{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (ethpb.Att, error) {
			return &ethpb.AttestationElectra{}, nil
		},
	}

	// Reset our aggregate attestation map.
	AggregateAttestationMap = map[[4]byte]func() (ethpb.SignedAggregateAttAndProof, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProofElectra{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProofElectra{}, nil
		},
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	altairSlot := primitives.Slot(params.BeaconConfig().AltairForkEpoch) * slotsPerEpoch
	bellatrixSlot := primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * slotsPerEpoch
	capellaSlot := primitives.Slot(params.BeaconConfig().CapellaForkEpoch) * slotsPerEpoch
	denebSlot := primitives.Slot(params.BeaconConfig().DenebForkEpoch) * slotsPerEpoch
	electraSlot := primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * slotsPerEpoch

	LightClientOptimisticUpdateMap = map[[4]byte]func() (interfaces.LightClientOptimisticUpdate, error){
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			m := &ethpb.LightClientOptimisticUpdateAltair{
				AttestedHeader: &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: altairSlot}},
			}
			return light_client.NewWrappedOptimisticUpdateAltair(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			m := &ethpb.LightClientOptimisticUpdateAltair{
				AttestedHeader: &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: bellatrixSlot}},
			}
			return light_client.NewWrappedOptimisticUpdateAltair(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			m := &ethpb.LightClientOptimisticUpdateCapella{
				AttestedHeader: &ethpb.LightClientHeaderCapella{Beacon: &ethpb.BeaconBlockHeader{Slot: capellaSlot}},
			}
			return light_client.NewWrappedOptimisticUpdateCapella(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			m := &ethpb.LightClientOptimisticUpdateDeneb{
				AttestedHeader: &ethpb.LightClientHeaderDeneb{Beacon: &ethpb.BeaconBlockHeader{Slot: denebSlot}},
			}
			return light_client.NewWrappedOptimisticUpdateDeneb(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			m := &ethpb.LightClientOptimisticUpdateDeneb{
				AttestedHeader: &ethpb.LightClientHeaderDeneb{Beacon: &ethpb.BeaconBlockHeader{Slot: electraSlot}},
			}
			return light_client.NewWrappedOptimisticUpdateDeneb(m)
		},
	}

	emptyExecutionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	emptyFinalityBranch := make([][]byte, fieldparams.FinalityBranchDepth)
	emptyFinalityBranchElectra := make([][]byte, fieldparams.FinalityBranchDepthElectra)
	for i := 0; i < len(emptyExecutionBranch); i++ {
		emptyExecutionBranch[i] = make([]byte, 32)
	}
	for i := 0; i < len(emptyFinalityBranch); i++ {
		emptyFinalityBranch[i] = make([]byte, 32)
	}
	for i := 0; i < len(emptyFinalityBranchElectra); i++ {
		emptyFinalityBranchElectra[i] = make([]byte, 32)
	}

	LightClientFinalityUpdateMap = map[[4]byte]func() (interfaces.LightClientFinalityUpdate, error){
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			m := &ethpb.LightClientFinalityUpdateAltair{
				AttestedHeader:  &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: altairSlot}},
				FinalizedHeader: &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: altairSlot}},
				FinalityBranch:  emptyFinalityBranch,
			}
			return light_client.NewWrappedFinalityUpdateAltair(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			m := &ethpb.LightClientFinalityUpdateAltair{
				AttestedHeader:  &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: bellatrixSlot}},
				FinalizedHeader: &ethpb.LightClientHeaderAltair{Beacon: &ethpb.BeaconBlockHeader{Slot: bellatrixSlot}},
				FinalityBranch:  emptyFinalityBranch,
			}
			return light_client.NewWrappedFinalityUpdateAltair(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			m := &ethpb.LightClientFinalityUpdateCapella{
				AttestedHeader: &ethpb.LightClientHeaderCapella{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: capellaSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderCapella{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalizedHeader: &ethpb.LightClientHeaderCapella{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: capellaSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderCapella{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalityBranch: emptyFinalityBranch,
			}
			return light_client.NewWrappedFinalityUpdateCapella(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			m := &ethpb.LightClientFinalityUpdateDeneb{
				AttestedHeader: &ethpb.LightClientHeaderDeneb{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: denebSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalizedHeader: &ethpb.LightClientHeaderDeneb{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: denebSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalityBranch: emptyFinalityBranch,
			}
			return light_client.NewWrappedFinalityUpdateDeneb(m)
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			m := &ethpb.LightClientFinalityUpdateElectra{
				AttestedHeader: &ethpb.LightClientHeaderDeneb{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: electraSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalizedHeader: &ethpb.LightClientHeaderDeneb{
					Beacon:          &ethpb.BeaconBlockHeader{Slot: electraSlot},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: emptyExecutionBranch,
				},
				FinalityBranch: emptyFinalityBranchElectra,
			}
			return light_client.NewWrappedFinalityUpdateElectra(m)
		},
	}
}
