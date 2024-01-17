package utils

import (
	"fmt"

	"github.com/iotaledger/iota.go/v4/api"
)

func SprintCommittee(resp *api.CommitteeResponse) string {
	t := fmt.Sprintf("Committee for epoch %d\n", resp.Epoch)
	t += fmt.Sprintf("Total Stake: %d\n", resp.TotalStake)
	t += fmt.Sprintf("Total Validators Stake: %d\n", resp.TotalValidatorStake)
	t += "----> Committee Members:\n"
	for _, member := range resp.Committee {
		t += SprintCommitteeMember(member)
	}
	t += "<----\n"

	return t
}

func SprintCommitteeMember(resp *api.CommitteeMemberResponse) string {
	t := fmt.Sprintf("Address Bech: %s\n", resp.AddressBech32)
	t += fmt.Sprintf("Pool Stake: %d\n", resp.PoolStake)
	t += fmt.Sprintf("Validator Stake: %d\n", resp.ValidatorStake)
	t += fmt.Sprintf("Fixed Cost: %d\n", resp.FixedCost)

	return t
}
