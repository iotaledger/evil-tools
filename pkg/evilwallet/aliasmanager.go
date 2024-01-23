package evilwallet

import (
	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
)

// region AliasManager /////////////////////////////////////////////////////////////////////////////////////////////////

// AliasManager is the manager for output aliases.
type AliasManager struct {
	outputMap map[string]*models.OutputData
	inputMap  map[string]*models.OutputData

	outputAliasCount *atomic.Uint64
	mu               syncutils.RWMutex
}

// NewAliasManager creates and returns a new AliasManager.
func NewAliasManager() *AliasManager {
	return &AliasManager{
		outputMap:        make(map[string]*models.OutputData),
		inputMap:         make(map[string]*models.OutputData),
		outputAliasCount: atomic.NewUint64(0),
	}
}

// AddOutputAlias maps the given outputAliasName to output, if there's duplicate outputAliasName, it will be overwritten.
func (a *AliasManager) AddOutputAlias(output *models.OutputData, aliasName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.outputMap[aliasName] = output
}

// AddInputAlias adds an input alias.
func (a *AliasManager) AddInputAlias(input *models.OutputData, aliasName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.inputMap[aliasName] = input
}

// GetInput returns the input for the alias specified.
func (a *AliasManager) GetInput(aliasName string) (*models.OutputData, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	in, ok := a.inputMap[aliasName]

	return in, ok
}

// GetOutput returns the output for the alias specified.
func (a *AliasManager) GetOutput(aliasName string) *models.OutputData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.outputMap[aliasName]
}

// ClearAllAliases clears all aliases.
func (a *AliasManager) ClearAllAliases() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.inputMap = make(map[string]*models.OutputData)
	a.outputMap = make(map[string]*models.OutputData)
}

// ClearAliases clears provided aliases.
func (a *AliasManager) ClearAliases(aliases ScenarioAlias) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, in := range aliases.Inputs {
		delete(a.inputMap, in)
	}
	for _, out := range aliases.Outputs {
		delete(a.outputMap, out)
	}
}

// AddOutputAliases batch adds the outputs their respective aliases.
func (a *AliasManager) AddOutputAliases(outputs []*models.OutputData, aliases []string) error {
	if len(outputs) != len(aliases) {
		return ierrors.New("mismatch outputs and aliases length")
	}
	for i, out := range outputs {
		a.AddOutputAlias(out, aliases[i])
	}

	return nil
}

// AddInputAliases batch adds the inputs their respective aliases.
func (a *AliasManager) AddInputAliases(inputs []*models.OutputData, aliases []string) error {
	if len(inputs) != len(aliases) {
		return ierrors.New("mismatch outputs and aliases length")
	}
	for i, out := range inputs {
		a.AddInputAlias(out, aliases[i])
	}

	return nil
}

// endregion /////////////////////////////////////////////////////////////////////////////////////////////////
