package runtime

import (
	"os"
	"path/filepath"
	"testing"

	configpkg "lazybase/internal/config"
	"lazybase/internal/ports"
	"lazybase/internal/project"
)

func TestPrepareLinksCustomSeedPathsFromSQLPaths(t *testing.T) {
	root := t.TempDir()
	supabaseDir := filepath.Join(root, "supabase")
	if err := os.MkdirAll(filepath.Join(supabaseDir, "seeds"), 0o755); err != nil {
		t.Fatalf("mkdir seeds dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(supabaseDir, "seeds", "001.sql"), []byte("select 1;\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	rawConfig := "[api]\nport = 54321\n\n[studio]\nport = 54323\n\n[db.seed]\nenabled = true\nsql_paths = [\"./seeds/*.sql\"]\n"
	configPath := filepath.Join(supabaseDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(rawConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := project.ResolveFromWorkingDir(root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	cfg, err := configpkg.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	targetPorts := ports.Compute(ports.Settings{Offset: ports.DefaultOffset}, 0, cfg.ActivePortKeys())
	if err := Prepare(info, cfg, targetPorts); err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	runtimeSeeds := filepath.Join(info.RuntimeSupabaseDir, "seeds")
	target, err := os.Readlink(runtimeSeeds)
	if err != nil {
		t.Fatalf("readlink runtime seeds: %v", err)
	}
	gotCanonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval symlink target %q: %v", target, err)
	}
	wantCanonical, err := filepath.EvalSymlinks(filepath.Join(supabaseDir, "seeds"))
	if err != nil {
		t.Fatalf("eval source seeds path: %v", err)
	}
	if gotCanonical != wantCanonical {
		t.Fatalf("expected runtime seeds symlink to source seeds; got %q want %q", gotCanonical, wantCanonical)
	}
}

func TestLinkCandidatesFromSQLPath(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		want        []string
		wantWarning string
	}{
		{name: "relative glob dir", in: "./seeds/*.sql", want: []string{"seeds"}},
		{name: "nested glob", in: "./db/seeds/**/*.sql", want: []string{filepath.Join("db", "seeds")}},
		{name: "single file", in: "./seed.sql", want: []string{"seed.sql"}},
		{name: "brace variants", in: "./seeds/{core,dev}/*.sql", want: []string{filepath.Join("seeds", "core"), filepath.Join("seeds", "dev")}},
		{name: "nested braces", in: "./seeds/{core,{dev,qa}}/*.sql", want: []string{filepath.Join("seeds", "core"), filepath.Join("seeds", "dev"), filepath.Join("seeds", "qa")}},
		{name: "absolute path ignored", in: "/tmp/seeds/*.sql", want: nil, wantWarning: "absolute paths are not supported"},
		{name: "path traversal ignored", in: "../seeds/*.sql", want: nil, wantWarning: "path traversal outside supabase dir is not allowed"},
		{name: "empty ignored", in: "  ", want: nil, wantWarning: "empty path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, warning := linkCandidatesFromSQLPath(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch for %q: got %d want %d (%#v)", tc.in, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("linkCandidatesFromSQLPath(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
			if warning != tc.wantWarning {
				t.Fatalf("warning mismatch for %q: got %q want %q", tc.in, warning, tc.wantWarning)
			}
		})
	}
}

func TestLinkedAssetNamesFromConfigIncludesBraceExpandedSeedPaths(t *testing.T) {
	rawConfig := []byte("[db.seed]\nsql_paths = [\"./seeds/{core,dev}/*.sql\"]\n")
	names, warnings := linkedAssetNamesFromConfig(rawConfig)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}

	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}

	if !has[filepath.Join("seeds", "core")] {
		t.Fatalf("expected seeds/core in linked assets: %#v", names)
	}
	if !has[filepath.Join("seeds", "dev")] {
		t.Fatalf("expected seeds/dev in linked assets: %#v", names)
	}
}

func TestLinkedAssetNamesFromConfigCollectsWarnings(t *testing.T) {
	rawConfig := []byte("[db.seed]\nsql_paths = [\"../seeds/*.sql\", \"/tmp/seed.sql\"]\n")
	_, warnings := linkedAssetNamesFromConfig(rawConfig)
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %#v", warnings)
	}
}
