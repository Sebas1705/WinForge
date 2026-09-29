// Command winforge-cli is the headless face of WinForge: scan this PC, plan or
// apply a profile, export what is installed. The desktop app uses the same
// packages.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/system"
)

const usage = `usage:
  winforge-cli scan                     list catalog apps installed on this PC
  winforge-cli profiles                 list catalog profiles
  winforge-cli plan <profile>           show what applying a profile would do
  winforge-cli apply <profile>          install what is missing, then run recipes
  winforge-cli export <id> [pin]        write this PC's catalog apps as a profile (stdout)
  winforge-cli check <file.json>        validate a profile file against the catalog`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "profiles":
		ids := make([]string, 0, len(cat.Profiles))
		for id := range cat.Profiles {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			p := cat.Profiles[id]
			fmt.Printf("%-16s %-5s %s\n", id, p.Kind, p.Name)
		}
	case "scan":
		installed := scan(ctx, cat)
		for _, id := range sortedInstalled(installed) {
			i := installed[id]
			fmt.Printf("%-28s %-16s %v\n", id, i.Version, i.Sources)
		}
		fmt.Printf("\n%d of %d catalog apps installed\n", len(installed), len(cat.Apps))
	case "plan", "apply":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf(usage))
		}
		res, err := cat.Resolve(os.Args[2])
		if err != nil {
			fatal(err)
		}
		plan := install.BuildPlan(cat, res, scan(ctx, cat))
		fmt.Printf("%d already installed, %d steps\n", len(plan.AlreadyInstalled), len(plan.Steps))
		for _, s := range plan.Steps {
			fmt.Printf("  %-7s %-28s %s\n", s.Kind, s.ID, adminMark(s.Admin))
		}
		for _, r := range plan.SkippedRecipes {
			fmt.Printf("  skipped recipe %s (its app is not part of this plan)\n", r)
		}
		if os.Args[1] == "apply" {
			failed := install.Run(ctx, cat, plan, install.OSExecutor{}, func(e install.Event) {
				switch e.Status {
				case install.StatusStart:
					fmt.Printf("-> %s %s\n", e.Step.Kind, e.Step.ID)
				case install.StatusOutput:
					fmt.Printf("   %s\n", e.Line)
				case install.StatusFailed, install.StatusSkipped:
					fmt.Printf("   %s: %s\n", e.Status, e.Err)
				}
			})
			if len(failed) > 0 {
				fmt.Println("failed:", failed)
				os.Exit(1)
			}
		}
	case "export":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf(usage))
		}
		installed := scan(ctx, cat)
		versions := map[string]string{}
		for id, i := range installed {
			versions[id] = i.Version
		}
		p := profile.FromInstalled(os.Args[2], os.Args[2], versions, len(os.Args) > 3 && os.Args[3] == "pin")
		b, err := profile.Export(p)
		if err != nil {
			fatal(err)
		}
		os.Stdout.Write(b)
	case "check":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf(usage))
		}
		b, err := os.ReadFile(os.Args[2])
		if err != nil {
			fatal(err)
		}
		imp, err := profile.Import(b, cat)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("ok: %q, %d apps, %d unknown apps, %d unknown recipes\n",
			imp.Profile.Name, len(imp.Profile.Apps), len(imp.UnknownApps), len(imp.UnknownRecipes))
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

func scan(ctx context.Context, cat *catalog.Catalog) map[string]system.Installed {
	inv, err := system.Snapshot(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	return system.Detect(cat, inv)
}

func sortedInstalled(m map[string]system.Installed) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func adminMark(a bool) string {
	if a {
		return "(needs admin)"
	}
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
