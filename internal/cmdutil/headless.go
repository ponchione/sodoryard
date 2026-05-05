package cmdutil

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ponchione/sodoryard/internal/headless"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	HeadlessExitOK             = int(headless.ExitOK)
	HeadlessExitInfrastructure = int(headless.ExitInfrastructure)
	HeadlessExitSafetyLimit    = int(headless.ExitSafetyLimit)
	HeadlessExitEscalation     = int(headless.ExitEscalation)
)

type HeadlessRunFlags struct {
	Role        string
	Task        string
	TaskFile    string
	ChainID     string
	MaxTurns    int
	MaxTokens   int
	Timeout     time.Duration
	ReceiptPath string
	Quiet       bool
	ProjectRoot string
}

type HeadlessRunResult struct {
	ReceiptPath string
	ExitCode    int
}

type HeadlessExitError struct {
	Code int
	Err  error
}

func (e HeadlessExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e HeadlessExitError) Unwrap() error { return e.Err }
func (e HeadlessExitError) ExitCode() int { return e.Code }

func RegisterHeadlessRunFlags(flags *pflag.FlagSet, values *HeadlessRunFlags) {
	flags.StringVar(&values.Role, "role", "", "Agent role config key or persona name")
	flags.StringVar(&values.Task, "task", "", "Task text for the headless run")
	flags.StringVar(&values.TaskFile, "task-file", "", "Read task text from file")
	flags.StringVar(&values.ChainID, "chain-id", "", "Chain execution identifier")
	flags.IntVar(&values.MaxTurns, "max-turns", 0, "Override max turns for this run")
	flags.IntVar(&values.MaxTokens, "max-tokens", 0, "Override max total tokens for this run")
	flags.DurationVar(&values.Timeout, "timeout", 0, "Wall-clock timeout for the entire session; 0 uses the role/default timeout")
	flags.StringVar(&values.ReceiptPath, "receipt-path", "", "Override brain-relative receipt path")
	flags.BoolVar(&values.Quiet, "quiet", false, "Suppress progress output")
	flags.StringVar(&values.ProjectRoot, "project-root", "", "Override project root")
}

func NewHeadlessRunCommand(use string, short string, configPath *string) *cobra.Command {
	flags := HeadlessRunFlags{}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := RunHeadlessForCommand(cmd, *configPath, flags)
			if result != nil && (result.ExitCode == HeadlessExitOK || result.ExitCode == HeadlessExitSafetyLimit) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.ReceiptPath)
			}
			if err != nil {
				return err
			}
			if result != nil && result.ExitCode != HeadlessExitOK {
				return HeadlessExitError{Code: result.ExitCode, Err: fmt.Errorf("headless run exited with code %d", result.ExitCode)}
			}
			return nil
		},
	}
	RegisterHeadlessRunFlags(cmd.Flags(), &flags)
	return cmd
}

func RunHeadlessForCommand(cmd *cobra.Command, configPath string, flags HeadlessRunFlags) (*HeadlessRunResult, error) {
	result, err := RunHeadless(cmd.Context(), cmd.ErrOrStderr(), configPath, flags)
	if err != nil {
		return nil, HeadlessExitError{Code: HeadlessExitInfrastructure, Err: err}
	}
	return result, nil
}

func RunHeadless(ctx context.Context, errOut io.Writer, configPath string, flags HeadlessRunFlags) (*HeadlessRunResult, error) {
	result, err := headless.RunSession(ctx, errOut, configPath, headless.RunRequest{
		Role:        flags.Role,
		Task:        flags.Task,
		TaskFile:    flags.TaskFile,
		ChainID:     flags.ChainID,
		MaxTurns:    flags.MaxTurns,
		MaxTokens:   flags.MaxTokens,
		Timeout:     flags.Timeout,
		ReceiptPath: flags.ReceiptPath,
		Quiet:       flags.Quiet,
		ProjectRoot: flags.ProjectRoot,
	}, headless.Deps{})
	if err != nil {
		return nil, err
	}
	return &HeadlessRunResult{ReceiptPath: result.ReceiptPath, ExitCode: int(result.ExitCode)}, nil
}
