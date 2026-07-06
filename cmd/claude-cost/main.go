// Command claude-cost reports Claude Code token usage and cost, computed
// from your local session transcripts (~/.claude/projects).
//
// Install:
//
//	go install github.com/sazardev/claude-go-expensive/cmd/claude-cost@latest
//
// Usage:
//
//	claude-cost [command] [flags]
//
// Commands: summary (default), projects, repos, sessions, files, folders.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	expensive "github.com/sazardev/claude-go-expensive"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claude-cost:", err)
		os.Exit(1)
	}
}

// splitCommand peels off the subcommand (if the first argument isn't itself
// a flag) before flag parsing, so "claude-cost sessions -limit 3" works the
// way every subcommand-flag CLI (git, docker, kubectl) does — flags scoped
// after their command, not before it. With no command, "summary" is the
// default and the whole argument list is left for flag parsing.
func splitCommand(args []string) (cmd string, rest []string) {
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return args[0], args[1:]
	}
	return "summary", args
}

func run(args []string) error {
	cmd, rest := splitCommand(args)

	switch cmd {
	case "help", "-h", "--help":
		usage()
		return nil
	case "version", "-version", "--version":
		fmt.Println("claude-cost", version())
		return nil
	}

	fs := flag.NewFlagSet("claude-cost "+cmd, flag.ExitOnError)
	dbPath := fs.String("db", "", "ruta a la base sqlite (default ~/.claude-cost/expensive.db)")
	logsDir := fs.String("logs", "", "directorio de transcripts de Claude Code (default ~/.claude/projects)")
	limit := fs.Int("limit", 10, "cuántas filas mostrar en sessions/files/folders")
	fs.Usage = usage
	if err := fs.Parse(rest); err != nil {
		return err
	}

	resolvedDB, err := resolveDBPath(*dbPath)
	if err != nil {
		return err
	}
	resolvedLogs := *logsDir
	if resolvedLogs == "" {
		if resolvedLogs, err = expensive.DefaultLogsDir(); err != nil {
			return err
		}
	}

	ctx := context.Background()
	tr, err := expensive.Open(resolvedDB)
	if err != nil {
		return fmt.Errorf("abrir base de datos: %w", err)
	}
	defer tr.Close()

	stats, err := tr.IngestDir(ctx, resolvedLogs)
	if err != nil {
		return fmt.Errorf("ingestar transcripts: %w", err)
	}
	if stats.Sessions > 0 {
		fmt.Printf("(%d sesiones actualizadas)\n\n", stats.Sessions)
	}
	for _, e := range stats.Errors {
		fmt.Fprintln(os.Stderr, "aviso:", e)
	}

	switch cmd {
	case "summary":
		return printSummary(ctx, tr)
	case "projects":
		return printProjects(ctx, tr)
	case "repos":
		return printRepos(ctx, tr)
	case "sessions":
		return printSessions(ctx, tr, *limit)
	case "files":
		return printFiles(ctx, tr, *limit)
	case "folders":
		return printFolders(ctx, tr, *limit)
	default:
		usage()
		return fmt.Errorf("comando desconocido: %s", cmd)
	}
}

// resolveDBPath defaults to a fixed, cwd-independent location so the same
// database is used no matter where the command is invoked from — the point
// of installing this as a real CLI instead of `go run` in a specific folder.
func resolveDBPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolver directorio home: %w", err)
	}
	dir := filepath.Join(home, ".claude-cost")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("crear %s: %w", dir, err)
	}
	return filepath.Join(dir, "expensive.db"), nil
}

// version reports the module version go install embedded, falling back to
// "(dev)" for a local `go run`/`go build` outside of `go install pkg@version`.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(dev)"
}

func usage() {
	fmt.Fprint(os.Stderr, `claude-cost — costo y uso de tokens de Claude Code, leído de tus transcripts locales.

Uso:
  claude-cost [comando] [flags]

Comandos:
  summary   (default) resumen total + costo por proyecto + top 5 sesiones
  projects  costo por proyecto
  repos     costo por repo/clon
  sessions  top sesiones más caras
  files     top archivos más caros
  folders   top carpetas más caras
  version   muestra la versión instalada

Flags:
  -db string      ruta a la base sqlite (default ~/.claude-cost/expensive.db)
  -logs string    directorio de transcripts (default ~/.claude/projects)
  -limit int      filas a mostrar en sessions/files/folders (default 10)
`)
}

func printSummary(ctx context.Context, tr *expensive.Tracker) error {
	summary, err := tr.Summary(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Total: $%.2f  ·  %d proyectos, %d sesiones, %d prompts\n\n",
		summary.CostUSD, summary.Projects, summary.Sessions, summary.Prompts)

	if err := printProjects(ctx, tr); err != nil {
		return err
	}
	fmt.Println()
	return printSessions(ctx, tr, 5)
}

func printProjects(ctx context.Context, tr *expensive.Tracker) error {
	rows, err := tr.CostByProject(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Por proyecto:")
	for _, p := range rows {
		fmt.Printf("  %-35s $%8.2f  (%d sesiones)\n", p.Project, p.CostUSD, p.Sessions)
	}
	return nil
}

func printRepos(ctx context.Context, tr *expensive.Tracker) error {
	rows, err := tr.CostByRepo(ctx)
	if err != nil {
		return err
	}
	fmt.Println("Por repo:")
	for _, r := range rows {
		fmt.Printf("  %-30s %-45s $%8.2f\n", r.Project, r.RepoRootPath, r.CostUSD)
	}
	return nil
}

func printSessions(ctx context.Context, tr *expensive.Tracker, limit int) error {
	rows, err := tr.CostBySession(ctx, limit)
	if err != nil {
		return err
	}
	fmt.Println("Sesiones más caras:")
	for _, s := range rows {
		fmt.Printf("  %-35s $%8.2f  %s\n", s.Project, s.CostUSD, s.StartedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

func printFiles(ctx context.Context, tr *expensive.Tracker, limit int) error {
	rows, err := tr.CostByFile(ctx, limit)
	if err != nil {
		return err
	}
	fmt.Println("Archivos más caros:")
	for _, f := range rows {
		fmt.Printf("  %-25s %-50s calls=%-3d $%.2f\n", f.Project, f.FilePath, f.ToolCalls, f.CostUSD)
	}
	return nil
}

func printFolders(ctx context.Context, tr *expensive.Tracker, limit int) error {
	rows, err := tr.CostByFolder(ctx, limit)
	if err != nil {
		return err
	}
	fmt.Println("Carpetas más caras:")
	for _, f := range rows {
		fmt.Printf("  %-25s %-40s calls=%-3d $%.2f\n", f.Project, f.FolderPath, f.ToolCalls, f.CostUSD)
	}
	return nil
}
