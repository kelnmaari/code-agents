package runner

// AgentRunner executes shell commands and returns their output.
type AgentRunner interface {
	Run(command string, env map[string]string) (output string, err error)
}

// RealRunner executes commands via the system shell.
type RealRunner struct {
	WorkDir string
}

// NewRealRunner creates a runner with the given working directory.
func NewRealRunner(workDir string) *RealRunner {
	return &RealRunner{WorkDir: workDir}
}
