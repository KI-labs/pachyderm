package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/spf13/cobra"
)

// RunFixedArgs wraps a function in a function
// that checks its exact argument count.
func RunFixedArgs(numArgs int, run func([]string) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		if len(args) != numArgs {
			fmt.Printf("expected %d arguments, got %d\n\n", numArgs, len(args))
			cmd.Usage()
		} else {
			if err := run(args); err != nil {
				ErrorAndExit("%v", err)
			}
		}
	}
}

// RunBoundedArgs wraps a function in a function
// that checks its argument count is within a range.
func RunBoundedArgs(min int, max int, run func([]string) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		if len(args) < min || len(args) > max {
			fmt.Printf("expected %d to %d arguments, got %d\n\n", min, max, len(args))
			cmd.Usage()
		} else {
			if err := run(args); err != nil {
				ErrorAndExit("%v", err)
			}
		}
	}
}

// Run makes a new cobra run function that wraps the given function.
func Run(run func(args []string) error) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, args []string) {
		if err := run(args); err != nil {
			ErrorAndExit(err.Error())
		}
	}
}

// ErrorAndExit errors with the given format and args, and then exits.
func ErrorAndExit(format string, args ...interface{}) {
	if errString := strings.TrimSpace(fmt.Sprintf(format, args...)); errString != "" {
		fmt.Fprintf(os.Stderr, "%s\n", errString)
	}
	os.Exit(1)
}

// ParseCommits takes a slice of arguments of the form "repo@branch-or-commit" or
// "repo" (in which case we consider the commit ID to be empty), and returns
// a list of *pfs.Commits
func ParseCommits(args []string) ([]*pfs.Commit, error) {
	var commits []*pfs.Commit
	for _, arg := range args {
		parts := strings.SplitN(arg, "@", 2)
		hasRepo := len(parts) > 0 && parts[0] != ""
		hasCommit := len(parts) == 2 && parts[1] != ""
		if hasCommit && !hasRepo {
			return nil, fmt.Errorf("invalid commit id \"%s\": repo cannot be empty", arg)
		}
		commit := &pfs.Commit{
			Repo: &pfs.Repo{
				Name: parts[0],
			},
		}
		if len(parts) == 2 {
			commit.ID = parts[1]
		} else {
			commit.ID = ""
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// ParseBranches takes a slice of arguments of the form "repo/branch-name" or
// "repo" (in which case we consider the branch name to be empty), and returns
// a list of *pfs.Branches
func ParseBranches(args []string) ([]*pfs.Branch, error) {
	commits, err := ParseCommits(args)
	if err != nil {
		return nil, err
	}
	var result []*pfs.Branch
	for _, commit := range commits {
		result = append(result, &pfs.Branch{Repo: commit.Repo, Name: commit.ID})
	}
	return result, nil
}

// RepeatedStringArg is an alias for []string
type RepeatedStringArg []string

func (r *RepeatedStringArg) String() string {
	result := "["
	for i, s := range *r {
		if i != 0 {
			result += ", "
		}
		result += s
	}
	return result + "]"
}

// Set adds a string to r
func (r *RepeatedStringArg) Set(s string) error {
	*r = append(*r, s)
	return nil
}

// Type returns the string representation of the type of r
func (r *RepeatedStringArg) Type() string {
	return "[]string"
}

// SetDocsUsage sets the usage string for a docs-style command.  Docs commands
// have no functionality except to output some docs and related commands, and
// should not specify a 'Run' attribute.
func SetDocsUsage(command *cobra.Command, subcommands []*cobra.Command) {
    command.SetUsageTemplate(`Usage:
  pachctl [command]{{if gt .Aliases 0}}

Aliases:
  {{.NameAndAliases}}
{{end}}{{if .HasExample}}

Examples:
{{ .Example }}{{end}}{{ if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{ if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsHelpCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}
`)

    command.SetHelpTemplate(`{{or .Long .Short}}
{{.UsageString}}`)

    // This song-and-dance is so that we can render the related commands without
    // actually having them usable as subcommands of the docs command.
    // That is, we don't want `pachctl job list-job` to work, it should just
    // be `pachctl list-job`.  Therefore, we lazily add/remove the subcommands
    // only when we try to render usage for the docs command.
    originalUsage := command.UsageFunc()
    command.SetUsageFunc(func (c *cobra.Command) error {
        newUsage := command.UsageFunc()
        command.SetUsageFunc(originalUsage)
        defer command.SetUsageFunc(newUsage)

        command.AddCommand(subcommands...)
        defer command.RemoveCommand(subcommands...)

        command.Usage()
        return nil
    })
}
