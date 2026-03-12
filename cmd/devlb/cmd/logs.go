package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

type logTarget struct {
	label   string
	logFile string
}

var logsCmd = &cobra.Command{
	Use:   "logs [label]",
	Short: "Show logs from exec-started backend processes",
	Long: `Display stdout/stderr captured from backends started with devlb exec.

Without arguments, shows logs from all active backends.
With a label argument, shows only that backend's logs.

Examples:
  devlb logs                  # all backends
  devlb logs feat-login       # specific label
  devlb logs -f               # follow mode (like tail -f)
  devlb logs -n 100           # last 100 lines
  devlb logs --port 3000      # filter by listen port`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getLabels(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		status, err := client.Status()
		if err != nil {
			return err
		}

		filterLabel := ""
		if len(args) > 0 {
			filterLabel = args[0]
		}
		filterPort, _ := cmd.Flags().GetInt("port")
		lines, _ := cmd.Flags().GetInt("lines")
		follow, _ := cmd.Flags().GetBool("follow")

		// Collect log files from backends
		var targets []logTarget

		for _, e := range status.Entries {
			if filterPort > 0 && e.ListenPort != filterPort {
				continue
			}
			for _, b := range e.Backends {
				if b.LogFile == "" {
					continue
				}
				if filterLabel != "" && b.Label != filterLabel {
					continue
				}
				// Avoid duplicates (same label may appear on multiple ports)
				dup := false
				for _, t := range targets {
					if t.logFile == b.LogFile {
						dup = true
						break
					}
				}
				if !dup {
					targets = append(targets, logTarget{label: b.Label, logFile: b.LogFile})
				}
			}
		}

		if len(targets) == 0 {
			if filterLabel != "" {
				return fmt.Errorf("no logs found for label %q", filterLabel)
			}
			return fmt.Errorf("no backends with log files found")
		}

		multi := len(targets) > 1

		if !follow {
			return showSnapshot(targets, lines, multi)
		}
		return followLogs(targets, lines, multi)
	},
}

// showSnapshot displays the last N lines from each log file.
func showSnapshot(targets []logTarget, lines int, multi bool) error {
	for _, t := range targets {
		tail, err := tailLines(t.logFile, lines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s: %v\n", t.logFile, err)
			continue
		}
		for _, line := range tail {
			if multi {
				fmt.Printf("[%s] %s\n", t.label, line)
			} else {
				fmt.Println(line)
			}
		}
	}
	return nil
}

// followLogs tails all log files concurrently, prefixing output with labels.
func followLogs(targets []logTarget, initialLines int, multi bool) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	var wg sync.WaitGroup
	done := make(chan struct{})

	for _, t := range targets {
		wg.Add(1)
		go func(lbl, path string) {
			defer wg.Done()
			tailFollow(lbl, path, initialLines, multi, done)
		}(t.label, t.logFile)
	}

	<-sigCh
	close(done)
	wg.Wait()
	return nil
}

func tailFollow(label, path string, initialLines int, multi bool, done <-chan struct{}) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	// Show initial lines
	lines, _ := tailLines(path, initialLines)
	for _, line := range lines {
		printLine(label, line, multi)
	}

	// Seek to end
	offset, _ := f.Seek(0, io.SeekEnd)

	buf := make([]byte, 4096)
	var partial string

	for {
		select {
		case <-done:
			return
		default:
		}

		n, err := f.ReadAt(buf, offset)
		if n > 0 {
			data := partial + string(buf[:n])
			partial = ""
			offset += int64(n)

			// Process complete lines
			for {
				idx := -1
				for i, c := range data {
					if c == '\n' {
						idx = i
						break
					}
				}
				if idx < 0 {
					partial = data
					break
				}
				line := data[:idx]
				data = data[idx+1:]
				printLine(label, line, multi)
			}
		}
		if err != nil && err != io.EOF {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func printLine(label, line string, multi bool) {
	if multi {
		fmt.Printf("[%s] %s\n", label, line)
	} else {
		fmt.Println(line)
	}
}

// tailLines reads the last n lines from a file.
func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var all []string
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (like tail -f)")
	logsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")
	logsCmd.Flags().Int("port", 0, "Filter by listen port")
	rootCmd.AddCommand(logsCmd)
}
