package utils

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
)

func SprintCommittee(resp *api.CommitteeResponse) string {
	t := fmt.Sprintf("Committee for epoch %d\n", resp.Epoch)
	t += fmt.Sprintf("Total Stake: %d\n", resp.TotalStake)
	t += fmt.Sprintf("Total Validators Stake: %d\n", resp.TotalValidatorStake)
	t += "Committee Members:\n"
	for _, member := range resp.Committee {
		t += SprintCommitteeMember(member)
	}
	t += "<----\n"

	return t
}

func SprintCommitteeMember(resp *api.CommitteeMemberResponse) string {
	t := "----> Committee Member:\n"
	t += fmt.Sprintf("Address Bech: %s\n", resp.AddressBech32)
	t += fmt.Sprintf("Pool Stake: %d\n", resp.PoolStake)
	t += fmt.Sprintf("Validator Stake: %d\n", resp.ValidatorStake)
	t += fmt.Sprintf("Fixed Cost: %d\n", resp.FixedCost)

	return t
}

func SprintValidators(resp *api.ValidatorsResponse) string {
	if len(resp.Validators) == 0 {
		return "There are no registered validators!"
	}
	t := "Validators: \n"
	for _, validator := range resp.Validators {
		t += SprintValidator(validator)
	}

	return t
}

func SprintValidator(resp *api.ValidatorResponse) string {
	t := "----> Validator:\n"
	t += fmt.Sprintf("Address Bech: %s\n", resp.AddressBech32)
	t += fmt.Sprintf("Staking Epoch End: %d\n", resp.StakingEndEpoch)
	t += fmt.Sprintf("Pool Stake: %d\n", resp.PoolStake)
	t += fmt.Sprintf("Validator Stake: %d\n", resp.ValidatorStake)
	t += fmt.Sprintf("Fixed Cost: %d\n", resp.FixedCost)
	t += fmt.Sprintf("Active: %v\n", resp.Active)

	return t
}

func SprintTransaction(api iotago.API, tx *iotago.SignedTransaction) string {
	jsonBytes, err := api.JSONEncode(tx)
	if err != nil {
		return ""
	}
	var out bytes.Buffer
	//nolint:errcheck
	json.Indent(&out, jsonBytes, "", "  ")

	txDetails := ""
	txDetails += fmt.Sprintf("\tSigned Transaction ID: %s, txID: %s, slotCreation: %d\n", lo.PanicOnErr(tx.ID()).ToHex(), lo.PanicOnErr(tx.Transaction.ID()).ToHex(), tx.Transaction.CreationSlot)

	txDetails += out.String()

	return txDetails
}

func SprintAccount(acc *iotago.AccountOutput) string {
	accountStr := ""
	accountStr += fmt.Sprintf("Account ID: %s\n", acc.AccountID.ToHex())
	accountStr += fmt.Sprintf("Account token balance: %d\n", acc.Amount)
	accountStr += fmt.Sprintf("Account stored mana: %d\n", acc.Mana)

	blockIssuerFeature := acc.FeatureSet().BlockIssuer()
	if blockIssuerFeature != nil {
		accountStr += fmt.Sprintf("Block Issuer Feature, number of keys: %d\n", len(blockIssuerFeature.BlockIssuerKeys))
	}
	stakingFeature := acc.FeatureSet().Staking()
	if stakingFeature != nil {
		accountStr += "Staking Feature, number of keys:\n"
		accountStr += fmt.Sprintf("Staked Amount: %d, Fixed Cost: %d, Start Epoch Start: %d, End Epoch: %d", stakingFeature.StakedAmount, stakingFeature.FixedCost, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
	}

	return accountStr
}

func SprintAvailableManaResult(results *builder.AvailableManaResult) string {
	return fmt.Sprintf("Available mana results:\nTotal: %d Unbound: %d\nPotential:%d Unbound: %d\nStored: %d Undound: %d", results.TotalMana, results.UnboundMana, results.PotentialMana, results.UnboundPotentialMana, results.StoredMana, results.UnboundStoredMana)
}
