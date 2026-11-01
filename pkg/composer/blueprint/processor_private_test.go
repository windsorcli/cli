package blueprint

import (
	"os"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/evaluator"
	"github.com/windsorcli/cli/pkg/runtime/shell"
)

// =============================================================================
// Test Setup
// =============================================================================

type ProcessorTestMocks struct {
	Shell         *shell.MockShell
	ConfigHandler *config.MockConfigHandler
	Evaluator     *evaluator.MockExpressionEvaluator
	Runtime       *runtime.Runtime
}

func setupProcessorMocks(t *testing.T) *ProcessorTestMocks {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

	mockShell := shell.NewMockShell()
	mockShell.GetProjectRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	mockConfigHandler := config.NewMockConfigHandler()
	mockConfigHandler.GetConfigRootFunc = func() (string, error) {
		return tmpDir, nil
	}

	realEvaluator := evaluator.NewExpressionEvaluator(mockConfigHandler, tmpDir, tmpDir)
	mockEvaluator := evaluator.NewMockExpressionEvaluator()

	mockEvaluator.EvaluateFunc = realEvaluator.Evaluate
	mockEvaluator.EvaluateMapFunc = realEvaluator.EvaluateMap
	mockEvaluator.SetTemplateDataFunc = realEvaluator.SetTemplateData
	mockEvaluator.RegisterFunc = realEvaluator.Register

	rt := &runtime.Runtime{
		Shims:         runtime.NewShims(),
		ProjectRoot:   tmpDir,
		ConfigRoot:    tmpDir,
		ConfigHandler: mockConfigHandler,
		Shell:         mockShell,
		Evaluator:     mockEvaluator,
	}

	mocks := &ProcessorTestMocks{
		Shell:         mockShell,
		ConfigHandler: mockConfigHandler,
		Evaluator:     mockEvaluator,
		Runtime:       rt,
	}

	t.Cleanup(func() {
		os.Unsetenv("WINDSOR_PROJECT_ROOT")
	})

	return mocks
}
