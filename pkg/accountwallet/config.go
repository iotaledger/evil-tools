package accountwallet

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

// commands

type AccountOperation int

const (
	OperationCreateAccount AccountOperation = iota
	OperationConvertAccount
	OperationDestroyAccount
	OperationAllotAccount
	OperationDelegateAccount
	OperationStakeAccount
	OperationRewards
	OperationListAccounts
	OperationUpdateAccount

	CmdNameCreateAccount   = "create"
	CmdNameConvertAccount  = "convert"
	CmdNameDestroyAccount  = "destroy"
	CmdNameAllotAccount    = "allot"
	CmdNameDelegateAccount = "delegate"
	CmdNameStakeAccount    = "stake"
	CmdNameRewards         = "rewards"
	CmdNameListAccounts    = "list"
	CmdNameUpdateAccount   = "update"
)

func (a AccountOperation) String() string {
	return []string{
		CmdNameCreateAccount,
		CmdNameConvertAccount,
		CmdNameDestroyAccount,
		CmdNameAllotAccount,
		CmdNameDelegateAccount,
		CmdNameStakeAccount,
		CmdNameRewards,
		CmdNameListAccounts,
		CmdNameUpdateAccount,
	}[a]
}

func AvailableCommands(cmd string) bool {
	availableCommands := map[string]types.Empty{
		CmdNameCreateAccount:   types.Void,
		CmdNameConvertAccount:  types.Void,
		CmdNameDestroyAccount:  types.Void,
		CmdNameAllotAccount:    types.Void,
		CmdNameDelegateAccount: types.Void,
		CmdNameStakeAccount:    types.Void,
		CmdNameRewards:         types.Void,
		CmdNameListAccounts:    types.Void,
		CmdNameUpdateAccount:   types.Void,
	}

	_, ok := availableCommands[cmd]

	return ok
}

type AccountSubcommands interface {
	Type() AccountOperation
}

type CreateAccountParams struct {
	Alias      string
	NoBIF      bool
	Implicit   bool
	Transition bool
}

func (c *CreateAccountParams) Type() AccountOperation {
	return OperationCreateAccount
}

type DestroyAccountParams struct {
	AccountAlias string
	ExpirySlot   uint64
}

func (d *DestroyAccountParams) Type() AccountOperation {
	return OperationDestroyAccount
}

type AllotAccountParams struct {
	Amount uint64
	To     string
}

func (a *AllotAccountParams) Type() AccountOperation {
	return OperationAllotAccount
}

type ConvertAccountParams struct {
	AccountAlias string
}

func (d *ConvertAccountParams) Type() AccountOperation {
	return OperationConvertAccount
}

type DelegateAccountParams struct {
	Amount    iotago.BaseToken
	ToAddress string
	FromAlias string
	CheckPool bool
}

func (a *DelegateAccountParams) Type() AccountOperation {
	return OperationDelegateAccount
}

type StakeAccountParams struct {
	Alias      string
	Amount     uint64
	FixedCost  uint64
	StartEpoch uint64
	EndEpoch   uint64
}

func (a *StakeAccountParams) Type() AccountOperation {
	return OperationStakeAccount
}

type RewardsAccountParams struct {
	Alias string
}

func (a *RewardsAccountParams) Type() AccountOperation {
	return OperationRewards
}

type UpdateAccountParams struct {
	Alias          string
	BlockIssuerKey string
	Mana           uint64
	Amount         uint64
	ExpirySlot     uint64
}

func (a *UpdateAccountParams) Type() AccountOperation {
	return OperationUpdateAccount
}

type NoAccountParams struct {
	Operation AccountOperation
}

func (a *NoAccountParams) Type() AccountOperation {
	return a.Operation
}

type StateData struct {
	Seed          string                 `serix:",lenPrefix=uint8"`
	LastUsedIndex uint32                 `serix:""`
	AccountsData  []*models.AccountState `serix:"accounts,lenPrefix=uint8"`
}
