package accountwallet

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ds/types"
	iotago "github.com/iotaledger/iota.go/v4"
)

// commands

type AccountOperation string

const (
	OperationCreateAccount   AccountOperation = "create"
	OperationConvertAccount  AccountOperation = "convert"
	OperationDestroyAccount  AccountOperation = "destroy"
	OperationAllotAccount    AccountOperation = "allot"
	OperationDelegateAccount AccountOperation = "delegate"
	OperationStakeAccount    AccountOperation = "stake"
	OperationRewardsAccount         AccountOperation = "rewards"
	OperationUpdateAccount   AccountOperation = "update"
)

func (a AccountOperation) String() string {
	return string(a)
}

func AvailableCommands(cmd string) bool {
	availableCommands := map[string]types.Empty{
		OperationCreateAccount.String():   types.Void,
		OperationConvertAccount.String():  types.Void,
		OperationDestroyAccount.String():  types.Void,
		OperationAllotAccount.String():    types.Void,
		OperationDelegateAccount.String(): types.Void,
		OperationStakeAccount.String():    types.Void,
		OperationRewardsAccount.String():    types.Void,
		OperationUpdateAccount.String():   types.Void,
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
	Wallets []*models.WalletState `serix:"wallets,lenPrefix=uint8"`
}
