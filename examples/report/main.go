// Command report is a runnable example: it ingests your local Claude Code
// session transcripts and prints a cost report.
//
// Usage:
//
//	go run ./examples/report              # ingest ~/.claude/projects
//	go run ./examples/report /some/dir    # ingest a specific directory
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	expensive "github.com/sazardev/claude-go-expensive"
)

func main() {
	ctx := context.Background()

	logsDir, err := expensive.DefaultLogsDir()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		logsDir = os.Args[1]
	}

	tr, err := expensive.Open("expensive.db")
	if err != nil {
		log.Fatal(err)
	}
	defer tr.Close()

	fmt.Println("ingesting:", logsDir)
	stats, err := tr.IngestDir(ctx, logsDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ingested %d sessions (%d already up to date)\n", stats.Sessions, stats.Skipped)
	for _, e := range stats.Errors {
		fmt.Println("  error:", e)
	}

	summary, err := tr.Summary(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%d projects, %d repos, %d sessions, %d prompts\n",
		summary.Projects, summary.Repos, summary.Sessions, summary.Prompts)
	fmt.Printf("tokens: in=%d out=%d cache5m=%d cache1h=%d cacheRead=%d\n",
		summary.InputTokens, summary.OutputTokens,
		summary.CacheCreation5mTokens, summary.CacheCreation1hTokens, summary.CacheReadTokens)
	fmt.Printf("total cost: $%.4f\n", summary.CostUSD)

	byProject, err := tr.CostByProject(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n--- cost by project ---")
	for _, p := range byProject {
		fmt.Printf("%-35s sessions=%-3d prompts=%-4d $%.4f\n", p.Project, p.Sessions, p.Prompts, p.CostUSD)
	}

	bySession, err := tr.CostBySession(ctx, 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n--- top 10 sessions ---")
	for _, s := range bySession {
		fmt.Printf("%-8s %-35s %s  prompts=%-4d $%.4f\n",
			s.SessionID[:min(8, len(s.SessionID))], s.Project, s.StartedAt.Format("2006-01-02 15:04"), s.Prompts, s.CostUSD)
	}

	byFile, err := tr.CostByFile(ctx, 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n--- top 10 files ---")
	for _, f := range byFile {
		fmt.Printf("%-35s %-60s calls=%-4d $%.4f\n", f.Project, f.FilePath, f.ToolCalls, f.CostUSD)
	}
}
