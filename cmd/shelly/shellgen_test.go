package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTopologicalInlineOrder: callee (lala) must appear before caller (baba).
func TestTopologicalInlineOrder(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src/lib", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile("src/lib/lala.sh", []byte("echo lala\n"), 0o644)
	os.WriteFile("src/lib/baba.sh", []byte("lala\n"), 0o644)

	cfg := &ShellyCfg{Name: "testapp", Version: "0.1.0"}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "testapp")
	idxL, idxB := strings.Index(s, "lala()"), strings.Index(s, "baba()")
	if idxL == -1 || idxB == -1 {
		t.Fatalf("missing functions: lala=%d baba=%d", idxL, idxB)
	}
	if idxL > idxB {
		t.Fatalf("lala(%d) should precede baba(%d)", idxL, idxB)
	}
}

// TestParserInjection: flag parser generated and invoked inside command.
func TestParserInjection(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "testapp2",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "upload", Flags: []Flag{{Long: "--user", Short: "-u", Arg: "user"}}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "testapp2")

	if !strings.Contains(s, "upload_parse()") {
		t.Fatal("missing upload_parse()")
	}
	if !strings.Contains(s, "upload_command()") {
		t.Fatal("missing upload_command()")
	}
	idx := strings.Index(s, "upload_command()")
	if !strings.Contains(s[idx:], "upload_parse \"$@\"") {
		t.Fatal("upload_command does not invoke upload_parse")
	}
}

// TestLibBodyOnly: lib body is wrapped as <stem>() { ... }.
func TestLibBodyOnly(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src/lib", 0o755)
	os.WriteFile("src/lib/greet.sh", []byte("echo \"hello $1\"\n"), 0o644)

	cfg := &ShellyCfg{Name: "myscript", Version: "1.0.0"}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "myscript")
	if !strings.Contains(s, "greet()") {
		t.Fatal("missing greet() wrapper")
	}
	if !strings.Contains(s, "echo \"hello $1\"") {
		t.Fatal("missing greet body")
	}
}

// TestCommandBodyOnly: command body is wrapped as <name>_command() { ... }.
func TestCommandBodyOnly(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)
	os.WriteFile("src/build_command.sh", []byte("echo building\nreturn 0\n"), 0o644)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.2.0",
		Commands: []Command{{Name: "build"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "build_command()") {
		t.Fatal("missing build_command()")
	}
	if !strings.Contains(s, "echo building") {
		t.Fatal("missing build body")
	}
}

// TestRequiredFlag: missing required flag causes error message in generated code.
func TestRequiredFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "push", Flags: []Flag{
				{Long: "--token", Arg: "token", Required: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "missing required flag: --token TOKEN") {
		t.Fatal("missing required-flag error message")
	}
}

// TestRequiredArg: missing required arg causes error message in generated code.
func TestRequiredArg(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Args: []Arg{
				{Name: "target", Required: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "missing required argument: TARGET") {
		t.Fatal("missing required-arg error message")
	}
}

// TestDefaultFlag: default value injection present in generated code.
func TestDefaultFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "serve", Flags: []Flag{
				{Long: "--port", Arg: "port", Default: "8080"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "${port:=8080}") {
		t.Fatal("missing default value injection for --port")
	}
}

// TestAllowedFlag: allowed-value validation present in generated code.
func TestAllowedFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "env", Flags: []Flag{
				{Long: "--env", Arg: "env", Allowed: []string{"prod", "stage", "dev"}},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "prod|stage|dev") {
		t.Fatal("missing allowed-value pattern for --env")
	}
}

// TestDepsCheck: missing dependency check in generated initialize().
func TestDepsCheck(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:         "mytool",
		Version:      "0.1.0",
		Dependencies: []Dependency{{Name: "jq", Help: "Install from https://stedolan.github.io/jq/"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "command -v jq") {
		t.Fatal("missing jq dependency check")
	}
}

// TestEnvVarCheck: required env var check in generated initialize().
func TestEnvVarCheck(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "API_KEY", Required: true, Help: "Set your API key"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "API_KEY") {
		t.Fatal("missing API_KEY env var check")
	}
	if !strings.Contains(s, "environment variable API_KEY is required") {
		t.Fatal("missing API_KEY required error message")
	}
}

// TestAutoGeneratedUsage: usage function references command names.
func TestAutoGeneratedUsage(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mycli",
		Version: "1.0.0",
		Help:    "My CLI tool",
		Commands: []Command{
			{Name: "build", Help: "Build the project"},
			{Name: "deploy", Help: "Deploy the project"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mycli")
	if !strings.Contains(s, "mycli_usage()") {
		t.Fatal("missing mycli_usage()")
	}
	if !strings.Contains(s, "Build the project") {
		t.Fatal("missing build help text in usage")
	}
	if !strings.Contains(s, "Deploy the project") {
		t.Fatal("missing deploy help text in usage")
	}
}

// TestMultipleAliases: all aliases route to the right command.
func TestMultipleAliases(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "download", Alias: "d", Aliases: []string{"dl", "fetch"}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	for _, alias := range []string{"d", "dl", "fetch"} {
		if !strings.Contains(s, alias+") download_command") {
			t.Fatalf("missing alias routing for %q", alias)
		}
	}
}

// TestCatchAll: remaining args captured into other_args variable.
func TestCatchAll(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", CatchAll: &CatchAllConfig{Label: "extra", Help: "Extra args", Required: false}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// parse_flags must accumulate non-flag args into other_args
	if !strings.Contains(s, "other_args") {
		t.Fatal("missing other_args variable in generated code")
	}
}

// TestConflicts: mutually exclusive flags produce error check in generated code.
func TestConflicts(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "build", Flags: []Flag{
				{Long: "--cache", Conflicts: []string{"--no-cache"}},
				{Long: "--no-cache"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "--cache") || !strings.Contains(s, "--no-cache") {
		t.Fatal("missing conflict flag names in generated code")
	}
	if !strings.Contains(s, "cannot be used together") {
		t.Fatal("missing conflict error message")
	}
}

// TestNeeds: co-required flags produce error check in generated code.
func TestNeeds(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "login", Flags: []Flag{
				{Long: "--user", Arg: "user"},
				{Long: "--password", Arg: "password", Needs: []string{"--user"}},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "requires --user") {
		t.Fatal("missing needs error message")
	}
}

// TestRepeatableFlag: repeatable boolean flag increments a counter; repeatable value flag accumulates.
func TestRepeatableFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--verbose", Short: "-v", Repeatable: true},
				{Long: "--file", Short: "-f", Arg: "file", Repeatable: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// boolean repeatable: should increment counter
	if !strings.Contains(s, "verbose=$((${verbose:-0}+1))") {
		t.Fatal("missing verbose counter increment")
	}
	// value repeatable: should accumulate into space-delimited string
	if !strings.Contains(s, "file=\"${file} $2\"") && !strings.Contains(s, "file=\"${file}") {
		t.Fatal("missing file accumulation")
	}
}

// TestRepeatableArg: repeatable positional arg accumulates remaining args.
func TestRepeatableArg(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "push", Args: []Arg{
				{Name: "files", Repeatable: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// repeatable arg should accumulate all remaining positionals
	if !strings.Contains(s, "files=\"$*\"") && !strings.Contains(s, "files=\"${files}") {
		t.Fatal("missing repeatable arg accumulation")
	}
}

// TestDefaultCommand: command with default:true catches unknown commands.
func TestDefaultCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "help"},
			{Name: "serve", DefaultCmd: DefaultField{isDefault: true}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// dispatcher *) wildcard should route to serve_command instead of error
	if !strings.Contains(s, "*) serve_command") {
		t.Fatal("missing default command routing in dispatcher")
	}
}

// TestPrivateCommand: private command is excluded from usage output.
func TestPrivateCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Help: "Deploy the app"},
			{Name: "internal", Help: "Internal only", Private: true},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Deploy the app") {
		t.Fatal("deploy command missing from usage")
	}
	if strings.Contains(s, "Internal only") {
		t.Fatal("private command should not appear in usage output")
	}
}

// TestFooter: footer text appears at end of root usage.
func TestFooter(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Footer:  "See https://example.com for more info.",
		Commands: []Command{
			{Name: "run"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "See https://example.com for more info.") {
		t.Fatal("missing footer text in usage")
	}
}

// TestGroup: group caption appears before grouped commands in usage.
func TestGroup(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "build", Help: "Build", Group: "Build Commands"},
			{Name: "test", Help: "Test", Group: "Build Commands"},
			{Name: "deploy", Help: "Deploy"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Build Commands") {
		t.Fatal("missing group caption in usage")
	}
}

// TestNestedGroup: nested (parent command) usage groups its sub-commands,
// matching the root usage behaviour. A parent's children with the same group
// must be listed under a "<group> Commands:" caption rather than a flat list.
func TestNestedGroup(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "install", Help: "Install things", Commands: []Command{
				{Name: "go", Help: "Go language", Group: "Language Development"},
				{Name: "zig", Help: "Zig language", Group: "Language Development"},
				{Name: "misc", Help: "Ungrouped tool"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Language Development Commands:") {
		t.Fatal("missing nested group caption in parent usage")
	}
	// The two grouped children must appear together inside the parent usage.
	if !strings.Contains(s, "  go ") || !strings.Contains(s, "  zig ") {
		t.Fatal("grouped nested commands missing from parent usage")
	}
	if out, err := exec.Command("sh", "-n", "./mytool").CombinedOutput(); err != nil {
		t.Fatalf("sh -n failed: %v\n%s", err, out)
	}
}

// TestPrivateFlag: private flag excluded from visible usage; inside reveal block only.
func TestPrivateFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Flags: []Flag{
				{Long: "--env", Arg: "env", Help: "Target environment"},
				{Long: "--debug-token", Arg: "token", Help: "Internal token", Private: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Target environment") {
		t.Fatal("public flag help missing from usage")
	}
	// "Internal token" must NOT appear before the reveal block
	revealIdx := strings.Index(s, "SHELLY_PRIVATE_REVEAL")
	beforeReveal := s
	if revealIdx != -1 {
		beforeReveal = s[:revealIdx]
	}
	if strings.Contains(beforeReveal, "Internal token") {
		t.Fatal("private flag help must not appear outside the reveal block")
	}
}

// TestDefaultForce: default:force command runs even when no args supplied.
func TestDefaultForce(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "help"},
			{Name: "serve", DefaultCmd: DefaultField{isDefault: true, isForce: true}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// with force, no-arg branch should invoke serve_command too
	if !strings.Contains(s, "serve_command") {
		t.Fatal("serve_command missing from generated script")
	}
	// the zero-arg branch must NOT call usage — it should call serve_command
	idx := strings.Index(s, "if [ $# -eq 0 ]")
	if idx == -1 {
		t.Fatal("missing zero-arg guard in run()")
	}
	zeroArgBlock := s[idx : idx+120]
	if strings.Contains(zeroArgBlock, "_usage") && !strings.Contains(zeroArgBlock, "serve_command") {
		t.Fatal("zero-arg branch should dispatch to force-default command, not show usage")
	}
}

// TestValidateFlag: validate on a flag injects a validation function call.
func TestValidateFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--port", Arg: "port", Validate: "integer"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer call for --port")
	}
}

// TestValidateArg: validate on an arg injects a validation function call.
func TestValidateArg(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Args: []Arg{
				{Name: "count", Validate: "integer"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer call for count arg")
	}
}

// TestValidateEnvVar: validate on an env var injects a validation function call.
func TestValidateEnvVar(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "TIMEOUT", Validate: "integer"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer call for TIMEOUT env var")
	}
}

// TestUniqueFlag: unique on repeatable flag deduplicates accumulated values.
func TestUniqueFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--tag", Arg: "tag", Repeatable: true, Unique: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "tag") {
		t.Fatal("missing tag variable in generated code")
	}
	// unique dedup: generated code must contain dedup logic referencing tag
	if !strings.Contains(s, "_dedup_tag") && !strings.Contains(s, "dedup") {
		t.Fatal("missing dedup logic for unique repeatable flag")
	}
}

// TestUniqueArg: unique on repeatable arg deduplicates accumulated values.
func TestUniqueArg(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Args: []Arg{
				{Name: "files", Repeatable: true, Unique: true},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "_dedup_files") && !strings.Contains(s, "dedup") {
		t.Fatal("missing dedup logic for unique repeatable arg")
	}
}

// TestAllowedEnvVar: allowed on env var injects whitelist check.
func TestAllowedEnvVar(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "APP_ENV", Allowed: []string{"prod", "stage", "dev"}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "prod|stage|dev") {
		t.Fatal("missing allowed-value pattern for APP_ENV")
	}
}

// TestPrivateEnvVar: private env var excluded from visible usage; inside reveal block only.
func TestPrivateEnvVar(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "API_KEY", Help: "Public API key"},
			{Name: "INTERNAL_TOKEN", Help: "Internal use only", Private: true},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Public API key") {
		t.Fatal("public env var help missing from usage")
	}
	// "Internal use only" must NOT appear before the reveal block
	revealIdx := strings.Index(s, "SHELLY_PRIVATE_REVEAL")
	beforeReveal := s
	if revealIdx != -1 {
		beforeReveal = s[:revealIdx]
	}
	if strings.Contains(beforeReveal, "Internal use only") {
		t.Fatal("private env var help must not appear outside the reveal block")
	}
}

// TestWildcardAlias: alias with trailing * matches commands by prefix.
func TestWildcardAlias(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "download", Alias: "d*"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// dispatcher must use d*) pattern for prefix-match routing
	if !strings.Contains(s, "d*) download_command") {
		t.Fatal("missing wildcard alias routing d*) download_command")
	}
}

// TestNestedSubcommands: nested commands generate parent dispatcher and child functions.
func TestNestedSubcommands(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "remote",
				Help: "Manage remotes",
				Commands: []Command{
					{Name: "add", Help: "Add a remote"},
					{Name: "remove", Help: "Remove a remote"},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// parent dispatcher function
	if !strings.Contains(s, "remote_command()") {
		t.Fatal("missing remote_command() dispatcher")
	}
	// child command functions
	if !strings.Contains(s, "remote_add_command()") {
		t.Fatal("missing remote_add_command()")
	}
	if !strings.Contains(s, "remote_remove_command()") {
		t.Fatal("missing remote_remove_command()")
	}
	// parent usage lists children
	if !strings.Contains(s, "Add a remote") {
		t.Fatal("missing child help text in parent usage")
	}
}

// TestRootFlagParsing: root-level flags generate a parser in root (no-subcommand) mode.
func TestRootFlagParsing(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Flags: []Flag{
			{Long: "--verbose", Short: "-v", Help: "Enable verbose output"},
			{Long: "--output", Short: "-o", Arg: "file", Help: "Output file", Default: "out.txt"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "root_parse()") {
		t.Fatal("missing root_parse()")
	}
	if !strings.Contains(s, "--verbose") {
		t.Fatal("missing --verbose in root flag parser")
	}
	if !strings.Contains(s, "${output:=out.txt}") {
		t.Fatal("missing default injection for --output")
	}
}

// TestRootArgParsing: root-level args generate a parser in root (no-subcommand) mode.
func TestRootArgParsing(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Args: []Arg{
			{Name: "source", Required: true, Help: "Source path"},
			{Name: "dest", Help: "Destination path"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "root_parse()") {
		t.Fatal("missing root_parse()")
	}
	if !strings.Contains(s, "missing required argument: SOURCE") {
		t.Fatal("missing required check for source arg")
	}
}

// TestCommandFooter: footer on a command appears in that command's usage.
func TestCommandFooter(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Help: "Deploy app", Footer: "See https://docs.example.com"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "See https://docs.example.com") {
		t.Fatal("missing command footer in generated usage")
	}
}

// TestCatchHelp: catch_all with catch_help shows other_args label in command usage.
func TestCatchHelp(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", CatchAll: &CatchAllConfig{
				Label:     "scripts",
				Help:      "Scripts to run",
				CatchHelp: true,
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "Scripts to run") {
		t.Fatal("missing catch_all help text in command usage")
	}
}

// TestDepORSyntax: dependency with multiple commands checks any one present.
func TestDepORSyntax(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Dependencies: []Dependency{
			{Commands: []string{"curl", "wget"}, Help: "Install curl or wget"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "curl") || !strings.Contains(s, "wget") {
		t.Fatal("missing OR-dep commands in generated code")
	}
	// must check both are absent before erroring, not just one
	if !strings.Contains(s, "command -v curl") {
		t.Fatal("missing command -v curl")
	}
	if !strings.Contains(s, "command -v wget") {
		t.Fatal("missing command -v wget")
	}
}

// TestValidateArray: validate as slice injects multiple validation calls.
func TestValidateArray(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--port", Arg: "port", ValidateList: []string{"integer", "not_empty"}},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer call")
	}
	if !strings.Contains(s, "validate_not_empty") {
		t.Fatal("missing validate_not_empty call")
	}
}

// TestDefaultArray: array default for repeatable flag injected as space-delimited string.
func TestDefaultArray(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--tag", Arg: "tag", Repeatable: true, DefaultList: []string{"latest", "stable"}},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "latest stable") {
		t.Fatal("missing array default as space-delimited string for --tag")
	}
}

// TestFilters: filters list injects filter function calls at top of command body.
func TestFilters(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Filters: []string{"require_git_clean", "require_docker"}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "require_git_clean") {
		t.Fatal("missing require_git_clean filter call")
	}
	if !strings.Contains(s, "require_docker") {
		t.Fatal("missing require_docker filter call")
	}
}

// TestExpose: expose:true shows child command list in parent help without --help.
func TestExpose(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:   "remote",
				Help:   "Manage remotes",
				Expose: NewExposeField(false),
				Commands: []Command{
					{Name: "add", Help: "Add a remote"},
					{Name: "remove", Help: "Remove a remote"},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// root usage must list the sub-subcommands (exposed)
	if !strings.Contains(s, "remote add") && !strings.Contains(s, "remote-add") {
		t.Fatal("exposed subcommands not visible in root usage")
	}
}

// TestVariables: variables inject named shell variable assignments into initialize().
func TestVariables(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Variables: []Variable{
			{Name: "config_dir", Value: NewVariableValue("${HOME}/.config/mytool")},
			{Name: "log_level", Value: NewVariableValue("info")},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "config_dir=") {
		t.Fatal("missing config_dir variable assignment")
	}
	if !strings.Contains(s, "log_level=") {
		t.Fatal("missing log_level variable assignment")
	}
}

// TestExtensible: extensible root tries PATH lookup for unknown sub-commands.
func TestExtensible(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:       "mytool",
		Version:    "0.1.0",
		Extensible: ExtensibleField{boolVal: true},
		Commands:   []Command{{Name: "build"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// dispatcher unknown-cmd branch should try mytool-<cmd> before erroring
	if !strings.Contains(s, "mytool-") {
		t.Fatal("missing extensible PATH lookup in dispatcher")
	}
}

// TestDefaultForceString: default: "force" YAML string value behaves like ForceDefault.
func TestDefaultForceString(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	// Simulate what YAML "default: force" unmarshals to via DefaultCmd field
	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "help"},
			{Name: "serve", DefaultCmd: DefaultField{isForce: true}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	idx := strings.Index(s, "if [ $# -eq 0 ]")
	if idx == -1 {
		t.Fatal("missing zero-arg guard in run()")
	}
	zeroArgBlock := s[idx : idx+120]
	if !strings.Contains(zeroArgBlock, "serve_command") {
		t.Fatal("zero-arg branch should dispatch to force-default command")
	}
}

// TestFunctionOverride: command with Function field uses custom function base name.
func TestFunctionOverride(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Function: "do_deploy"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "do_deploy_command()") {
		t.Fatal("missing do_deploy_command() function")
	}
	if !strings.Contains(s, "do_deploy_parse()") {
		t.Fatal("missing do_deploy_parse() function")
	}
}

var usageDefRe = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)\(\)`)
var usageRefRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*_usage)\b`)

// assertUsageRefsDefined fails if any <name>_usage function is referenced but
// never defined. Every command parser calls its usage function on error/help, so
// an undefined reference is a runtime bug.
func assertUsageRefsDefined(t *testing.T, s string) {
	t.Helper()
	defs := map[string]bool{}
	for _, m := range usageDefRe.FindAllStringSubmatch(s, -1) {
		defs[m[1]] = true
	}
	for _, m := range usageRefRe.FindAllStringSubmatch(s, -1) {
		if m[1] == "long_usage" {
			continue // bashly-style flag variable, not a usage function
		}
		if !defs[m[1]] {
			t.Fatalf("usage function %q is referenced but not defined", m[1])
		}
	}
}

// assertValidFuncNames fails if the script defines any function whose name is not
// a valid POSIX shell identifier (e.g. contains a hyphen).
func assertValidFuncNames(t *testing.T, s string) {
	t.Helper()
	for _, m := range usageDefRe.FindAllStringSubmatch(s, -1) {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(m[1]) {
			t.Fatalf("invalid shell function name %q", m[1])
		}
	}
}

// TestHyphenatedCommandName: a command whose name contains a hyphen must generate
// valid shell identifiers (hyphens become underscores) while the user-facing name
// keeps the hyphen. Regression test for "Bad function name" syntax errors.
func TestHyphenatedCommandName(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "go-blueprint", Args: []Arg{{Name: "dir"}}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	for _, want := range []string{
		"go_blueprint_parse()",
		"go_blueprint_command()",
		"mytool_go_blueprint_usage()",
		"go-blueprint) go_blueprint_command",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in generated script", want)
		}
	}
	for _, bad := range []string{"go-blueprint_parse()", "go-blueprint_command()", "go-blueprint_usage()"} {
		if strings.Contains(s, bad) {
			t.Fatalf("generated script contains invalid function definition %q", bad)
		}
	}
	assertValidFuncNames(t, s)
	assertUsageRefsDefined(t, s)
	if out, err := exec.Command("sh", "-n", "./mytool").CombinedOutput(); err != nil {
		t.Fatalf("sh -n failed: %v\n%s", err, out)
	}
}

// TestBodyScaffold: missing body files are created with bashly's scaffold and
// that scaffold is embedded verbatim in the generated script.
func TestBodyScaffold(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Commands: []Command{{Name: "run", Help: "Run"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	want := "echo \"# This file is located at 'src/run_command.sh'.\"\n" +
		"echo \"# It contains the implementation for the 'mytool run' command.\"\n" +
		"echo \"# The code you write here will be wrapped by a function named 'mytool_run_command()'.\"\n" +
		"echo \"# Feel free to edit this file; your changes will persist when regenerating.\"\n" +
		"inspect_args\n"
	got, err := os.ReadFile("src/run_command.sh")
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != want {
		t.Fatalf("scaffold mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Editing the body must survive regeneration.
	if err := os.WriteFile("src/run_command.sh", []byte("echo custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen (2): %v", err)
	}
	got, _ = os.ReadFile("src/run_command.sh")
	if string(got) != "echo custom\n" {
		t.Fatalf("regeneration clobbered edited body: %q", got)
	}
}

// TestBodyScaffoldHyphenated: the file path/display keep the raw command name
// while the wrapped function name is sanitized to a valid shell identifier.
func TestBodyScaffoldHyphenated(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Commands: []Command{{Name: "go-blueprint", Help: "Blueprint"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	got, err := os.ReadFile("src/go-blueprint_command.sh")
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, want := range []string{
		"'src/go-blueprint_command.sh'",
		"'mytool go-blueprint' command",
		"'mytool_go_blueprint_command()'",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("scaffold missing %q:\n%s", want, got)
		}
	}
}

// TestBodyScaffoldRoot: root-only mode uses src/root_command.sh, displays only
// the app name, and wraps in root_command().
func TestBodyScaffoldRoot(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "rootonly",
		Version: "0.1.0",
		Args:    []Arg{{Name: "file"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	got, err := os.ReadFile("src/root_command.sh")
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, want := range []string{
		"'src/root_command.sh'",
		"'rootonly' command",
		"'root_command()'",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("scaffold missing %q:\n%s", want, got)
		}
	}
}

// TestNestedCommandUsageDefined: nested leaf commands must each get a usage
// function, otherwise their parsers reference an undefined <app>_<path>_usage.
func TestNestedCommandUsageDefined(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "remote",
				Commands: []Command{
					{Name: "add", Args: []Arg{{Name: "url", Required: true}}},
					{Name: "remove"},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	for _, want := range []string{
		"mytool_remote_add_usage()",
		"mytool_remote_remove_usage()",
		"mytool_remote_usage()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in generated script", want)
		}
	}
	assertUsageRefsDefined(t, s)
	if out, err := exec.Command("sh", "-n", "./mytool").CombinedOutput(); err != nil {
		t.Fatalf("sh -n failed: %v\n%s", err, out)
	}
}

// TestFunctionOverrideUsageDefined: a command using function: override must have
// its usage function named after the override, matching what the parser calls.
func TestFunctionOverrideUsageDefined(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Function: "do_deploy", Args: []Arg{{Name: "env", Required: true}}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "mytool_do_deploy_usage()") {
		t.Fatal("missing mytool_do_deploy_usage()")
	}
	assertUsageRefsDefined(t, s)
	if out, err := exec.Command("sh", "-n", "./mytool").CombinedOutput(); err != nil {
		t.Fatalf("sh -n failed: %v\n%s", err, out)
	}
}

// TestFilenameOverride: command with Filename field reads body from custom path.
func TestFilenameOverride(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	// write body at the custom filename
	customBody := "echo custom_deploy_body\nreturn 0\n"
	if err := os.WriteFile("src/custom_deploy.sh", []byte(customBody), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Filename: "src/custom_deploy.sh"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "custom_deploy_body") {
		t.Fatal("body from custom filename not included")
	}
}

// TestCommandVariables: command-scoped variables injected at top of command body.
func TestCommandVariables(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "deploy", Variables: []Variable{
				{Name: "deploy_env", Value: NewVariableValue("production")},
				{Name: "deploy_timeout", Value: NewVariableValue("30")},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "deploy_env=\"production\"") {
		t.Fatal("missing deploy_env variable in command body")
	}
	if !strings.Contains(s, "deploy_timeout=\"30\"") {
		t.Fatal("missing deploy_timeout variable in command body")
	}
}

// TestShortOnlyFlag: flag with only Short (no Long) is parsed correctly.
func TestShortOnlyFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Short: "-v", Help: "Verbose"},
				{Short: "-o", Arg: "file", Help: "Output file"},
			}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "-v)") {
		t.Fatal("missing -v short-only flag case")
	}
	if !strings.Contains(s, "-o)") {
		t.Fatal("missing -o short-only flag case")
	}
}

// TestBeforeAfterHooks: src/before.sh and src/after.sh emit wrapped functions called from run().
func TestBeforeAfterHooks(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	os.WriteFile("src/before.sh", []byte("echo before_hook\n"), 0o644)
	os.WriteFile("src/after.sh", []byte("echo after_hook\n"), 0o644)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Commands: []Command{{Name: "build"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// hook functions must be defined
	if !strings.Contains(s, "before_hook() {") {
		t.Fatal("missing before_hook() function definition")
	}
	if !strings.Contains(s, "after_hook() {") {
		t.Fatal("missing after_hook() function definition")
	}
	// hook body must be indented inside the function
	if !strings.Contains(s, "  echo before_hook") {
		t.Fatal("before_hook body not properly indented")
	}
	if !strings.Contains(s, "  echo after_hook") {
		t.Fatal("after_hook body not properly indented")
	}
	// functions defined before run(), in order: before_hook, after_hook, initialize, run
	beforeFuncIdx := strings.Index(s, "before_hook() {")
	afterFuncIdx := strings.Index(s, "after_hook() {")
	initIdx := strings.Index(s, "initialize() {")
	runIdx := strings.Index(s, "run() {")
	if beforeFuncIdx > afterFuncIdx {
		t.Fatal("before_hook() should appear before after_hook()")
	}
	if afterFuncIdx > initIdx {
		t.Fatal("after_hook() should appear before initialize()")
	}
	if initIdx > runIdx {
		t.Fatal("initialize() should appear before run()")
	}
	// run() body must call the hooks, not inline them
	if !strings.Contains(s, "  before_hook\n") {
		t.Fatal("run() should call before_hook")
	}
	if !strings.Contains(s, "  after_hook\n") {
		t.Fatal("run() should call after_hook")
	}
}

// TestDepsArray: when dependencies present, $deps array populated in initialize().
func TestDepsArray(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Dependencies: []Dependency{
			{Name: "curl"},
			{Name: "jq"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// deps array should be set in initialize or generateDepsCheck
	if !strings.Contains(s, "deps_curl=") && !strings.Contains(s, "deps[curl]") && !strings.Contains(s, "deps_curl") {
		t.Fatal("missing deps_curl path assignment")
	}
	if !strings.Contains(s, "deps_jq") {
		t.Fatal("missing deps_jq path assignment")
	}
}

// TestDepVersion: dependency with Version field emits version check.
func TestDepVersion(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Dependencies: []Dependency{
			{Name: "docker", Version: "20.0"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "20.0") {
		t.Fatal("missing version requirement in deps check")
	}
}

// TestEnvVarValidateList: EnvironmentVariable with ValidateList injects multiple validate_ calls.
func TestEnvVarValidateList(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "LOG_LEVEL", ValidateList: []string{"not_empty", "integer"}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "validate_not_empty") {
		t.Fatal("missing validate_not_empty call for env var")
	}
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer call for env var")
	}
}

// TestPreview: shellGen to stdout, not file.
func TestPreview(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Commands: []Command{{Name: "build", Help: "Build it"}},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "#!/bin/sh") {
		t.Fatal("missing shebang in preview output")
	}
	if !strings.Contains(out, "build_command()") {
		t.Fatal("missing build_command() in preview output")
	}
	// preview must NOT create file on disk
	if _, err := os.Stat("mytool"); err == nil {
		t.Fatal("preview must not write file to disk")
	}
}

// TestValidateSemantics: validate detects duplicate command names and orphaned needs/conflicts.
func TestValidateSemantics(t *testing.T) {
	// duplicate command names
	cfg := &ShellyCfg{
		Name: "mytool",
		Commands: []Command{
			{Name: "build"},
			{Name: "build"}, // duplicate
		},
	}
	errs := cfg.validateSemantics()
	if len(errs) == 0 {
		t.Fatal("expected error for duplicate command name 'build'")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "build") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error mentioning 'build', got: %v", errs)
	}

	// orphaned needs: --bar needs --baz but --baz not defined
	cfg2 := &ShellyCfg{
		Name: "mytool",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--bar", Needs: []string{"--baz"}},
			}},
		},
	}
	errs2 := cfg2.validateSemantics()
	if len(errs2) == 0 {
		t.Fatal("expected error for orphaned needs --baz")
	}
	found2 := false
	for _, e := range errs2 {
		if strings.Contains(e, "baz") {
			found2 = true
		}
	}
	if !found2 {
		t.Fatalf("expected error mentioning 'baz', got: %v", errs2)
	}

	// orphaned conflicts: --foo conflicts with --qux but --qux not defined
	cfg3 := &ShellyCfg{
		Name: "mytool",
		Commands: []Command{
			{Name: "run", Flags: []Flag{
				{Long: "--foo", Conflicts: []string{"--qux"}},
			}},
		},
	}
	errs3 := cfg3.validateSemantics()
	if len(errs3) == 0 {
		t.Fatal("expected error for orphaned conflict --qux")
	}
	found3 := false
	for _, e := range errs3 {
		if strings.Contains(e, "qux") {
			found3 = true
		}
	}
	if !found3 {
		t.Fatalf("expected error mentioning 'qux', got: %v", errs3)
	}
}

// TestExtensibleDelegate: extensible: "git" delegates unknown commands to git.
func TestExtensibleDelegate(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:       "mytool",
		Version:    "0.1.0",
		Extensible: ExtensibleField{delegateVal: "git"},
		Commands:   []Command{{Name: "build"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// unknown commands dispatched to: git "$_cmd" "$@"
	if !strings.Contains(s, "git \"$_cmd\"") && !strings.Contains(s, "git $_cmd") {
		t.Fatal("missing git delegate in run dispatcher")
	}
}

// TestShowExamplesOnError: required arg missing prints examples in usage.
func TestShowExamplesOnError(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:     "deploy",
				Examples: []string{"mytool deploy production"},
				Args:     []Arg{{Name: "env", Required: true}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// when <env> is missing, usage (which contains example) is called
	if !strings.Contains(s, "mytool_deploy_usage") {
		t.Fatal("missing usage call on required arg error")
	}
}

// TestPrivateRevealKey: private commands visible when SHELLY_PRIVATE_REVEAL env var set.
func TestPrivateRevealKey(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "build", Help: "Build it"},
			{Name: "secret", Help: "Internal command", Private: true},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// usage must contain conditional: if SHELLY_PRIVATE_REVEAL is set, show secret
	if !strings.Contains(s, "SHELLY_PRIVATE_REVEAL") {
		t.Fatal("missing SHELLY_PRIVATE_REVEAL check in usage")
	}
	if !strings.Contains(s, "secret") {
		t.Fatal("missing secret command in generated output")
	}
}

// helper
func readGenerated(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestUnifiedExtensibleString: extensible: "git" via single YAML key delegates unknown cmds.
func TestUnifiedExtensibleString(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
extensible: git
commands:
  - name: build
    help: Build it
`
	cfg := mustParseYAML(t, yaml)
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg.Name = "mytool"
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "git \"$_cmd\"") && !strings.Contains(s, "git $_cmd") {
		t.Fatal("expected git delegate in run dispatcher")
	}
}

// TestUnifiedExtensibleBool: extensible: true via single YAML key uses PATH lookup.
func TestUnifiedExtensibleBool(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
extensible: true
commands:
  - name: build
    help: Build it
`
	cfg := mustParseYAML(t, yaml)
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg.Name = "mytool"
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "mytool-\"$_cmd\"") && !strings.Contains(s, "mytool-$_cmd") {
		t.Fatal("expected PATH-based extensible lookup in run dispatcher")
	}
}

// TestUnifiedDefault: default: force via YAML key on a command triggers zero-arg dispatch.
func TestUnifiedDefault(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
commands:
  - name: serve
    help: Serve it
    default: force
`
	cfg := mustParseYAML(t, yaml)
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg.Name = "mytool"
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// zero-arg branch must dispatch to serve_command
	if !strings.Contains(s, "serve_command") {
		t.Fatal("expected serve_command in run dispatcher")
	}
	if !strings.Contains(s, "[ $# -eq 0 ]") {
		t.Fatal("expected zero-arg guard in run dispatcher")
	}
}

// TestPrivateRevealKeyConfig: private_reveal_key replaces hardcoded SHELLY_PRIVATE_REVEAL.
func TestPrivateRevealKeyConfig(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:             "mytool",
		Version:          "0.1.0",
		PrivateRevealKey: "MYTOOL_REVEAL",
		Commands: []Command{
			{Name: "build", Help: "Build it"},
			{Name: "secret", Help: "Internal", Private: true},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "MYTOOL_REVEAL") {
		t.Fatal("expected custom private_reveal_key MYTOOL_REVEAL in usage")
	}
	// default key should NOT appear when overridden
	if strings.Contains(s, "SHELLY_PRIVATE_REVEAL") {
		t.Fatal("should not contain default SHELLY_PRIVATE_REVEAL when overridden")
	}
}

// TestHeaderSh: src/header.sh content injected after shebang.
func TestHeaderSh(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	os.WriteFile("src/header.sh", []byte("# my custom header\n# generated by CI\n"), 0o644)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Commands: []Command{{Name: "build", Help: "Build it"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "my custom header") {
		t.Fatal("expected header.sh content in generated script")
	}
	// must appear before version_command
	headerIdx := strings.Index(s, "my custom header")
	versionIdx := strings.Index(s, "version_command()")
	if headerIdx > versionIdx {
		t.Fatal("header.sh content must appear before version_command")
	}
}

// TestStrictMode: Strict:true emits set -euo pipefail and IFS in initialize.
func TestStrictMode(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		Strict:   StrictField{boolVal: true},
		Commands: []Command{{Name: "build", Help: "Build it"}},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "set -euo pipefail") {
		t.Fatal("expected set -euo pipefail in strict mode")
	}
	if !strings.Contains(s, "IFS") {
		t.Fatal("expected IFS assignment in strict mode")
	}
}

// TestArgfile: argfile:true emits argfile loading block in run() or initialize().
func TestArgfile(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:    "deploy",
				Help:    "Deploy it",
				Argfile: ArgfileField{boolVal: true},
				Flags:   []Flag{{Long: "--env", Arg: "environment", Help: "Target env"}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// argfile: generates code that reads .mytool-deploy flags from file
	if !strings.Contains(s, "argfile") && !strings.Contains(s, ".mytool") && !strings.Contains(s, "ARGFILE") {
		t.Fatal("expected argfile loading code in generated script")
	}
}

// TestGlobalFlags: flags on a parent command with subcommands are parsed before dispatch.
func TestGlobalFlags(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Flags: []Flag{
			{Long: "--verbose", Short: "-v", Help: "Enable verbose output"},
		},
		Commands: []Command{
			{Name: "build", Help: "Build it"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	// global flags are parsed inline in run() and peeled off before dispatch.
	runIdx := strings.Index(s, "\nrun() {")
	if runIdx == -1 {
		t.Fatal("missing run()")
	}
	runBody := s[runIdx:]
	flagIdx := strings.Index(runBody, "--verbose)")
	dispatchIdx := strings.Index(runBody, "build) ")
	if flagIdx == -1 {
		t.Fatal("run() must parse the global flag --verbose")
	}
	if dispatchIdx == -1 {
		t.Fatal("run() must dispatch the build command")
	}
	if flagIdx > dispatchIdx {
		t.Fatal("global flags must be parsed before command dispatch")
	}
}

// helper: parse YAML string into ShellyCfg
func mustParseYAML(t *testing.T, y string) *ShellyCfg {
	t.Helper()
	var cfg ShellyCfg
	if err := parseYAML([]byte(y), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	return &cfg
}

// TestAddValidations: shelly add validations writes src/lib/validations.sh with validate_ functions.
func TestAddValidations(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)

	if err := addAddon("validations", false); err != nil {
		t.Fatalf("addAddon validations: %v", err)
	}
	b, err := os.ReadFile("src/lib/validations.sh")
	if err != nil {
		t.Fatal("src/lib/validations.sh not created")
	}
	s := string(b)
	if !strings.Contains(s, "validate_not_empty") {
		t.Fatal("missing validate_not_empty in validations.sh")
	}
	if !strings.Contains(s, "validate_integer") {
		t.Fatal("missing validate_integer in validations.sh")
	}
	if !strings.Contains(s, "validate_file_exists") {
		t.Fatal("missing validate_file_exists in validations.sh")
	}
}

// TestAddColors: shelly add colors writes src/lib/colors.sh with ANSI helpers.
func TestAddColors(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)

	if err := addAddon("colors", false); err != nil {
		t.Fatalf("addAddon colors: %v", err)
	}
	b, err := os.ReadFile("src/lib/colors.sh")
	if err != nil {
		t.Fatal("src/lib/colors.sh not created")
	}
	s := string(b)
	if !strings.Contains(s, "red") && !strings.Contains(s, "RED") {
		t.Fatal("missing red color in colors.sh")
	}
}

// TestAddHooks: shelly add hooks writes src/before.sh and src/after.sh stubs.
func TestAddHooks(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	if err := addAddon("hooks", false); err != nil {
		t.Fatalf("addAddon hooks: %v", err)
	}
	if _, err := os.Stat("src/before.sh"); err != nil {
		t.Fatal("src/before.sh not created")
	}
	if _, err := os.Stat("src/after.sh"); err != nil {
		t.Fatal("src/after.sh not created")
	}
}

// TestAddUnknown: unknown addon returns an error.
func TestAddUnknown(t *testing.T) {
	if err := addAddon("nonexistent", false); err == nil {
		t.Fatal("expected error for unknown addon")
	}
}

// TestArgfileLineByLine: argfile preamble must read file line-by-line, skip
// blanks and comments, and only prepend lines starting with '-'.
// Must NOT use the broken set -- "$(cat ...)" pattern.
func TestArgfileLineByLine(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:    "deploy",
				Help:    "Deploy it",
				Argfile: ArgfileField{boolVal: true},
				Flags:   []Flag{{Long: "--env", Arg: "environment", Help: "Target env"}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// must NOT use the broken single-string cat pattern
	if strings.Contains(s, `set -- "$(cat`) {
		t.Fatal("argfile preamble uses broken set -- \"$(cat ...) pattern; must use line-by-line read")
	}
	// must use while/read loop for line-by-line parsing
	if !strings.Contains(s, "while") || !strings.Contains(s, "read") {
		t.Fatal("argfile preamble must use a while/read loop for line-by-line parsing")
	}
	// must skip comment lines (lines starting with #)
	if !strings.Contains(s, "#") {
		// loose check: the generated comment-skip code references '#'
		t.Fatal("argfile preamble missing comment-skip logic")
	}
	// must only prepend lines starting with '-'
	if !strings.Contains(s, `case "$`) {
		t.Fatal("argfile preamble must filter lines with a case statement or similar")
	}
}

// TestCatchHelpPassthrough: when catch_help:true, -h/--help must fall through
// to other_args accumulation instead of calling usage and exiting.
func TestCatchHelpPassthrough(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "run",
				Help: "Run something",
				CatchAll: &CatchAllConfig{
					Label:     "command",
					CatchHelp: true,
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find the run_parse function
	start := strings.Index(s, "run_parse()")
	if start == -1 {
		t.Fatal("missing run_parse()")
	}
	// find the end of run_parse (next function)
	end := strings.Index(s[start:], "\nrun_command()")
	if end == -1 {
		end = len(s) - start
	}
	parserBody := s[start : start+end]

	// When catch_help is true, -h|--help must NOT call usage and exit
	// It should fall into other_args accumulation instead
	if strings.Contains(parserBody, "-h|--help) mytool_run_usage") {
		t.Fatal("catch_help:true should NOT intercept -h|--help with usage call; it must fall through to other_args")
	}
	// The -h and --help args should land in other_args
	if !strings.Contains(parserBody, "other_args") {
		t.Fatal("run_parse missing other_args (catch_all required for catch_help)")
	}
}

// TestDoubleDashPositionals: '--' must switch the parser into positional mode
// so tokens after it are treated as positionals (single-pass parsing).
func TestDoubleDashPositionals(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:  "run",
				Help:  "Run something",
				Flags: []Flag{{Long: "--verbose", Short: "-v", Help: "Verbose"}},
				Args:  []Arg{{Name: "target", Required: true}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// The command must invoke the single parser exactly once.
	start := strings.Index(s, "run_command()")
	if start == -1 {
		t.Fatal("missing run_command()")
	}
	cmdBody := s[start:]
	if !strings.Contains(cmdBody[:strings.Index(cmdBody, "}")], `run_parse "$@"`) {
		t.Fatal("run_command must call run_parse \"$@\"")
	}

	// The two-pass parser must be gone.
	if strings.Contains(s, "run_parse_args") || strings.Contains(s, "parse_flags") {
		t.Fatal("two-pass parse_flags/parse_args must be replaced by a single parser")
	}

	// The single parser handles '--' by switching to positional mode.
	pStart := strings.Index(s, "run_parse()")
	if pStart == -1 {
		t.Fatal("missing run_parse()")
	}
	pEnd := strings.Index(s[pStart:], "\nrun_command()")
	if pEnd == -1 {
		pEnd = len(s) - pStart
	}
	parserBody := s[pStart : pStart+pEnd]
	if !strings.Contains(parserBody, "--) shift; _options_ended=true") {
		t.Fatal("parser must treat '--' by enabling positional mode")
	}
	// The positional case must consume '$1' inside the same loop.
	if !strings.Contains(parserBody, "_pos=$((_pos + 1))") {
		t.Fatal("parser must track positional index in the same loop")
	}
}

// TestSinglePassRuntimePositions executes a generated script and verifies the
// core issue-0001 behaviors: flags and positionals interleave, and '--' forces
// positional treatment.
func TestSinglePassRuntimePositions(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
      - long: --output
        short: -o
        arg: file
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh",
		[]byte(`echo "source=[$source] force=[$force] output=[$output]"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	// flag before positional
	if got := run("download", "--force", "example.com"); !strings.Contains(got, "source=[example.com]") || !strings.Contains(got, "force=[1]") {
		t.Fatalf("flag-before-positional failed: %q", got)
	}
	// flag after positional
	if got := run("download", "example.com", "--force"); !strings.Contains(got, "source=[example.com]") || !strings.Contains(got, "force=[1]") {
		t.Fatalf("flag-after-positional failed: %q", got)
	}
	// '--' makes the following token a positional, even if it looks like a flag
	if got := run("download", "--", "--force"); !strings.Contains(got, "source=[--force]") || !strings.Contains(got, "force=[]") {
		t.Fatalf("double-dash positional failed: %q", got)
	}
	// inline --flag=value for a value flag
	if got := run("download", "--output=out.txt", "x"); !strings.Contains(got, "source=[x]") || !strings.Contains(got, "output=[out.txt]") {
		t.Fatalf("inline value failed: %q", got)
	}
	// short flag with separate value, interleaved with positional
	if got := run("download", "x", "-o", "y"); !strings.Contains(got, "source=[x]") || !strings.Contains(got, "output=[y]") {
		t.Fatalf("short value flag failed: %q", got)
	}
}

// TestFlagAliasRuntime: every spelling (canonical long/short and aliases) sets
// the same canonical variable, for both boolean and value flags.
func TestArgfilePathMode(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    argfile: ./download.args
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
      - long: --output
        short: -o
        arg: file
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh",
		[]byte(`echo "source=[$source] force=[$force] output=[$output]"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	// argfile in CWD: comment line, boolean flag, same-line quoted value,
	// and an unknown flag that must be ignored.
	os.WriteFile("download.args",
		[]byte("# defaults\n--force\n--output \"out dir.txt\"\n--unknown ignored\n"), 0o644)

	out, err := exec.Command("sh", "./myapp", "download", "x").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"source=[x]", "force=[1]", "output=[out dir.txt]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestArgfileBoolBackCompat(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    argfile: true
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh",
		[]byte(`echo "source=[$source] force=[$force]"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	// legacy path: ${HOME}/.config/myapp/download
	if err := os.MkdirAll(dir+"/.config/myapp", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir+"/.config/myapp/download", []byte("--force\n"), 0o644)

	cmd := exec.Command("sh", "./myapp", "download", "x")
	cmd.Env = append(os.Environ(), "HOME="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "force=[1]") || !strings.Contains(got, "source=[x]") {
		t.Errorf("unexpected output:\n%s", got)
	}
}

func TestFlagAliasRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
        alias: -F
        aliases: [--forcefully, --forcibly]
      - long: --output
        short: -o
        aliases: [--out]
        arg: file
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh",
		[]byte(`echo "force=[$force] output=[$output]"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	for _, spelling := range []string{"--force", "-f", "-F", "--forcefully", "--forcibly"} {
		if got := run("download", spelling, "x"); !strings.Contains(got, "force=[1]") {
			t.Fatalf("alias %s did not set force: %q", spelling, got)
		}
	}
	for _, spelling := range []string{"--output", "-o", "--out"} {
		if got := run("download", spelling, "y", "x"); !strings.Contains(got, "output=[y]") {
			t.Fatalf("alias %s did not set output: %q", spelling, got)
		}
	}
}

// TestFlagAliasHelpOutput: aliases appear in the generated usage/help text.
func TestFlagAliasHelpOutput(t *testing.T) {
	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    flags:
      - long: --force
        short: -f
        aliases: [-F, --forcefully]
        help: Force it
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	for _, want := range []string{"-f", "--force", "-F", "--forcefully"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing alias spelling %q", want)
		}
	}
}

// TestFlagAliasConflictsNeeds: conflicts/needs that reference an alias resolve
// to the canonical flag.
func TestFlagAliasConflictsNeeds(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
        aliases: [-F]
      - long: --dry-run
        short: -n
        conflicts: [-F]
      - long: --verbose
        short: -v
        needs: [-F]
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh", []byte("echo ok\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	run := func(args ...string) (string, error) {
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		return string(out), err
	}

	// conflict: force (-F alias) with --dry-run must fail
	if out, err := run("download", "-F", "-n", "x"); err == nil || !strings.Contains(out, "cannot be used together") {
		t.Fatalf("alias conflict not detected: err=%v out=%q", err, out)
	}
	// needs satisfied: --verbose with force present (-F alias)
	if out, err := run("download", "-F", "-v", "x"); err != nil {
		t.Fatalf("needs unexpectedly failed: %v\n%s", err, out)
	}
	// needs unsatisfied: --verbose without force must fail
	if out, err := run("download", "-v", "x"); err == nil || !strings.Contains(out, "requires") {
		t.Fatalf("needs not detected: err=%v out=%q", err, out)
	}
}

// TestFlagNegatableRuntime: --no-<flag> unsets a negatable boolean flag.
func TestFlagNegatableRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: run
    help: Run it
    flags:
      - long: --color
        short: -c
        negatable: true
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/run_command.sh", []byte("echo \"color=[$color]\"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if got := run("run", "--color"); !strings.Contains(got, "color=[1]") {
		t.Fatalf("--color did not set value: %q", got)
	}
	if got := run("run", "--no-color"); !strings.Contains(got, "color=[]") {
		t.Fatalf("--no-color did not unset value: %q", got)
	}
	if got := run("run", "-c"); !strings.Contains(got, "color=[1]") {
		t.Fatalf("-c did not set value: %q", got)
	}
}

// TestFlagNegatableHelpOutput: help shows the --[no-] form.
func TestFlagNegatableHelpOutput(t *testing.T) {
	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: run
    help: Run it
    flags:
      - long: --color
        short: -c
        negatable: true
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "--[no-]color") {
		t.Fatal("help output missing --[no-]color")
	}
}

// TestFlagNegatableValidation: invalid negatable combinations are rejected.
func TestFlagNegatableValidation(t *testing.T) {
	cases := []struct {
		name  string
		flags []Flag
	}{
		{"arg", []Flag{{Long: "--color", Negatable: true, Arg: "mode"}}},
		{"repeatable", []Flag{{Long: "--color", Negatable: true, Repeatable: true}}},
		{"required", []Flag{{Long: "--color", Negatable: true, Required: true}}},
		{"no-long", []Flag{{Short: "-c", Negatable: true}}},
	}
	for _, tc := range cases {
		cfg := &ShellyCfg{
			Name:     "myapp",
			Commands: []Command{{Name: "run", Flags: tc.flags}},
		}
		if errs := cfg.validateSemantics(); len(errs) == 0 {
			t.Fatalf("negatable+%s: expected validation error, got none", tc.name)
		}
	}
	ok := &ShellyCfg{
		Name:     "myapp",
		Commands: []Command{{Name: "run", Flags: []Flag{{Long: "--color", Negatable: true}}}},
	}
	if errs := ok.validateSemantics(); len(errs) != 0 {
		t.Fatalf("valid negatable reported errors: %v", errs)
	}
}

// TestCommandPrefixAliasRuntime: a trailing "*" alias matches any token with
// that prefix and dispatches to the owning command.
func TestCommandPrefixAliasRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: foo
    help: Foo command
    alias: "fo*"
  - name: bar
    help: Bar command
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/foo_command.sh", []byte(`echo "ran foo"`+"\n"), 0o644)
	os.WriteFile("src/bar_command.sh", []byte(`echo "ran bar"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	for _, tok := range []string{"fo", "foobar", "foo-x"} {
		if got := run(tok); !strings.Contains(got, "ran foo") {
			t.Fatalf("prefix alias %q did not dispatch to foo: %q", tok, got)
		}
	}
	if got := run("bar"); !strings.Contains(got, "ran bar") {
		t.Fatalf("exact name bar failed: %q", got)
	}
	if out, err := exec.Command("sh", "./myapp", "xyz").CombinedOutput(); err == nil {
		t.Fatalf("unknown command should exit non-zero, got: %q", out)
	}
}

// TestCommandPrefixAliasExactPriority: exact command names/aliases take priority
// over prefix aliases (exact arms are emitted before prefix arms).
func TestCommandPrefixAliasExactPriority(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: foo
    help: Foo command
  - name: fizz
    help: Fizz command
    alias: "fo*"
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/foo_command.sh", []byte(`echo "ran foo"`+"\n"), 0o644)
	os.WriteFile("src/fizz_command.sh", []byte(`echo "ran fizz"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	// "foo" is an exact name and also matches the "fo*" prefix of fizz.
	if got := run("foo"); !strings.Contains(got, "ran foo") {
		t.Fatalf("exact name should beat prefix alias: %q", got)
	}
	// Non-exact tokens fall through to the prefix alias.
	for _, tok := range []string{"fo", "foobar"} {
		if got := run(tok); !strings.Contains(got, "ran fizz") {
			t.Fatalf("prefix alias %q should dispatch to fizz: %q", tok, got)
		}
	}
}

// TestCommandPrefixAliasValidation: validateSemantics rejects malformed prefix
// aliases (empty, equal to own name, or overlapping with another command).
func TestCommandPrefixAliasValidation(t *testing.T) {
	cases := []struct {
		name string
		cmds []Command
	}{
		{"empty", []Command{{Name: "foo", Alias: "*"}}},
		{"own-name", []Command{{Name: "fo", Alias: "fo*"}}},
		{"ambiguous", []Command{
			{Name: "a", Alias: "fo*"},
			{Name: "b", Alias: "foobar*"},
		}},
	}
	for _, tc := range cases {
		cfg := &ShellyCfg{Name: "myapp", Commands: tc.cmds}
		if errs := cfg.validateSemantics(); len(errs) == 0 {
			t.Fatalf("prefix alias %s: expected validation error, got none", tc.name)
		}
	}
	ok := &ShellyCfg{Name: "myapp", Commands: []Command{{Name: "foo", Alias: "fo*"}}}
	if errs := ok.validateSemantics(); len(errs) != 0 {
		t.Fatalf("valid prefix alias reported errors: %v", errs)
	}
}

// TestExposeAlwaysYAML: expose:"always" parsed correctly from YAML (not bool).
func TestExposeAlwaysYAML(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
commands:
  - name: remote
    help: Manage remotes
    expose: always
    commands:
      - name: add
        help: Add a remote
      - name: remove
        help: Remove a remote
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	remote := cfg.Commands[0]
	if !remote.Expose.IsAlways() {
		t.Fatalf("expose:always should have IsAlways()=true, got Expose=%+v", remote.Expose)
	}
	if !remote.Expose.IsEnabled() {
		t.Fatal("expose:always should have IsEnabled()=true")
	}
}

// TestExposeBoolYAML: expose:true still works after refactor to ExposeField.
func TestExposeBoolYAML(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
commands:
  - name: remote
    help: Manage remotes
    expose: true
    commands:
      - name: add
        help: Add a remote
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	remote := cfg.Commands[0]
	if !remote.Expose.IsEnabled() {
		t.Fatal("expose:true should have IsEnabled()=true")
	}
	if remote.Expose.IsAlways() {
		t.Fatal("expose:true should have IsAlways()=false")
	}
}

// TestExposeAlwaysDispatch: expose:"always" causes the sub-dispatcher to show
// subcommands inline (not just call usage) when no args are given.
func TestExposeAlwaysDispatch(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: mytool
version: 0.1.0
commands:
  - name: remote
    help: Manage remotes
    expose: always
    commands:
      - name: add
        help: Add a remote
      - name: remove
        help: Remove a remote
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find remote_command dispatcher body
	start := strings.Index(s, "remote_command()")
	if start == -1 {
		t.Fatal("missing remote_command()")
	}
	dispBody := s[start:]

	// With expose:always, no-arg path must echo subcommands inline, not only delegate to remote_usage.
	// Extract the zero-arg branch: from "if [ $# -eq 0 ]" to "fi"
	zeroArgStart := strings.Index(dispBody, "if [ $# -eq 0 ]")
	if zeroArgStart == -1 {
		t.Fatal("expose:always dispatcher missing zero-arg branch")
	}
	zeroArgEnd := strings.Index(dispBody[zeroArgStart:], "fi\n")
	if zeroArgEnd == -1 {
		zeroArgEnd = strings.Index(dispBody[zeroArgStart:], "fi")
	}
	zeroArgBranch := dispBody[zeroArgStart : zeroArgStart+zeroArgEnd+3]
	// must contain echo output for the subcommands in the zero-arg path
	if !strings.Contains(zeroArgBranch, "echo") {
		t.Fatal("expose:always zero-arg branch must echo subcommand list, not only call usage")
	}
	if !strings.Contains(zeroArgBranch, "add") || !strings.Contains(zeroArgBranch, "remove") {
		t.Fatal("expose:always zero-arg branch must list subcommands (add, remove) inline")
	}
}

// TestVariableArrayError: Variable with a YAML array value must return an error
// during config parsing (POSIX sh has no arrays).
func TestVariableArrayError(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
variables:
  - name: MY_LIST
    value:
      - one
      - two
`
	var cfg ShellyCfg
	err := parseYAML([]byte(yaml), &cfg)
	if err == nil {
		t.Fatal("expected error when Variable.value is a YAML array, got nil")
	}
}

// TestVariableHashError: Variable with a YAML map value must return an error.
func TestVariableHashError(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
variables:
  - name: MY_MAP
    value:
      key: val
`
	var cfg ShellyCfg
	err := parseYAML([]byte(yaml), &cfg)
	if err == nil {
		t.Fatal("expected error when Variable.value is a YAML map, got nil")
	}
}

// TestVariableStringOk: Variable with a plain string value must parse without error.
func TestVariableStringOk(t *testing.T) {
	yaml := `
name: mytool
version: 0.1.0
variables:
  - name: MY_VAR
    value: hello
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("expected no error for string variable, got: %v", err)
	}
	if len(cfg.Variables) == 0 || cfg.Variables[0].Value.String() != "hello" {
		t.Fatal("Variable value not parsed correctly")
	}
}

// TestPrivateFlagReveal: private flags are hidden by default but shown when
// the reveal env var is set.
func TestPrivateFlagReveal(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "deploy",
				Help: "Deploy",
				Flags: []Flag{
					{Long: "--env", Arg: "env", Help: "Target environment"},
					{Long: "--secret", Arg: "secret", Help: "Secret flag", Private: true},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find mytool_deploy_usage
	start := strings.Index(s, "mytool_deploy_usage()")
	if start == -1 {
		t.Fatal("missing mytool_deploy_usage()")
	}
	usageBody := s[start : start+strings.Index(s[start:], "\n}")]

	// --secret must NOT appear outside reveal block
	// It should only appear inside an if [ -n "${SHELLY_PRIVATE_REVEAL}" ] block
	beforeReveal := usageBody
	revealIdx := strings.Index(usageBody, "SHELLY_PRIVATE_REVEAL")
	if revealIdx != -1 {
		beforeReveal = usageBody[:revealIdx]
	}
	if strings.Contains(beforeReveal, "--secret") {
		t.Fatal("private flag --secret must not be shown outside the reveal block")
	}
	// The reveal block must exist and contain --secret
	if revealIdx == -1 {
		t.Fatal("usage must contain SHELLY_PRIVATE_REVEAL reveal block for private flags")
	}
	if !strings.Contains(usageBody[revealIdx:], "--secret") {
		t.Fatal("private flag --secret must appear inside the SHELLY_PRIVATE_REVEAL reveal block")
	}
}

// TestPrivateEnvVarReveal: private env vars are hidden by default but shown
// when the reveal env var is set.
func TestPrivateEnvVarReveal(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "deploy",
				Help: "Deploy",
				EnvironmentVariables: []EnvironmentVariable{
					{Name: "APP_ENV", Help: "Target environment"},
					{Name: "APP_SECRET", Help: "Secret env var", Private: true},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	start := strings.Index(s, "mytool_deploy_usage()")
	if start == -1 {
		t.Fatal("missing mytool_deploy_usage()")
	}
	usageBody := s[start : start+strings.Index(s[start:], "\n}")]

	revealIdx := strings.Index(usageBody, "SHELLY_PRIVATE_REVEAL")
	beforeReveal := usageBody
	if revealIdx != -1 {
		beforeReveal = usageBody[:revealIdx]
	}
	if strings.Contains(beforeReveal, "APP_SECRET") {
		t.Fatal("private env var APP_SECRET must not be shown outside the reveal block")
	}
	if revealIdx == -1 {
		t.Fatal("usage must contain SHELLY_PRIVATE_REVEAL reveal block for private env vars")
	}
	if !strings.Contains(usageBody[revealIdx:], "APP_SECRET") {
		t.Fatal("private env var APP_SECRET must appear inside the SHELLY_PRIVATE_REVEAL reveal block")
	}
}

// ─── word_wrap tests ──────────────────────────────────────────────────────────

// TestWrapTextBasic: wrapText splits at word boundary before the column limit.
func TestWrapTextBasic(t *testing.T) {
	// 40-char wrap, text > 40 chars
	text := "This is a somewhat long help description that should be wrapped"
	got := wrapText(text, 40)
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 40 {
			t.Fatalf("line exceeds 40 chars: %q (len=%d)", line, len(line))
		}
	}
	// must preserve all words
	if strings.ReplaceAll(got, "\n", " ") != text {
		t.Fatalf("wrapText lost/changed words:\n got:  %q\n want: %q", strings.ReplaceAll(got, "\n", " "), text)
	}
}

// TestWrapTextShort: text shorter than limit is returned unchanged (no newline added).
func TestWrapTextShort(t *testing.T) {
	text := "Short help"
	got := wrapText(text, 80)
	if got != text {
		t.Fatalf("wrapText changed short text: got %q", got)
	}
}

// TestWrapTextZero: width=0 disables wrapping, returns text unchanged.
func TestWrapTextZero(t *testing.T) {
	text := "This is a very long help string that would normally need wrapping but should not be wrapped"
	got := wrapText(text, 0)
	if got != text {
		t.Fatalf("wrapText with width=0 should be a no-op, got %q", got)
	}
}

// TestWrapTextExactBoundary: a word that starts exactly at the limit should wrap.
func TestWrapTextExactBoundary(t *testing.T) {
	// "12345678 abcd" at width=8: "12345678" fits, "abcd" must be on next line
	text := "12345678 abcd"
	got := wrapText(text, 8)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrap into 2 lines, got 1: %q", got)
	}
	if lines[0] != "12345678" {
		t.Fatalf("first line wrong: %q", lines[0])
	}
	if lines[1] != "abcd" {
		t.Fatalf("second line wrong: %q", lines[1])
	}
}

// TestWordWrapInUsage: when word_wrap is set, long flag/arg/env help strings
// are split across multiple echo lines in generated usage functions.
func TestWordWrapInUsage(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	longHelp := "This is a very long help description for this flag that definitely exceeds eighty characters when combined with the flag name prefix"

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		WordWrap: WordWrapField{width: 80, explicit: true},
		Commands: []Command{
			{
				Name: "deploy",
				Help: "Deploy the application",
				Flags: []Flag{
					{Long: "--target", Arg: "env", Help: longHelp},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find the usage function
	start := strings.Index(s, "mytool_deploy_usage()")
	if start == -1 {
		t.Fatal("missing mytool_deploy_usage()")
	}
	usageEnd := strings.Index(s[start:], "\n}")
	usageBody := s[start : start+usageEnd]

	// no single echo line should exceed 80 chars of content
	for _, line := range strings.Split(usageBody, "\n") {
		// strip leading spaces + echo " prefix
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "echo \"") {
			// extract content between outer quotes
			inner := strings.TrimPrefix(trimmed, "echo \"")
			inner = strings.TrimSuffix(inner, "\"")
			if len(inner) > 80 {
				t.Fatalf("echo content exceeds 80 chars (%d): %q", len(inner), inner)
			}
		}
	}

	// the long help string must appear split across multiple echo lines
	// (i.e. longHelp does NOT appear as one contiguous string)
	if strings.Contains(s, longHelp) {
		t.Fatal("word_wrap:80 set but long help string was not split")
	}
}

// TestWordWrapDefaultsTo80: when word_wrap is unset, help text wraps at 80
// so a long help string is split across echo lines.
func TestWordWrapDefaultsTo80(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	longHelp := "This is a very long help description that should be wrapped because word_wrap defaults to eighty"

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:  "run",
				Help:  "Run it",
				Flags: []Flag{{Long: "--mode", Arg: "m", Help: longHelp}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if strings.Contains(s, longHelp) {
		t.Fatal("word_wrap unset should wrap at 80, but the long help string was emitted raw")
	}
}

// TestWordWrapDisabled: an explicit word_wrap of 0 (or false) disables wrapping,
// so a long help string is emitted raw.
func TestWordWrapDisabled(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	longHelp := "This is a very long help description that should not be wrapped because word_wrap is explicitly disabled"

	cfg := &ShellyCfg{
		Name:     "mytool",
		Version:  "0.1.0",
		WordWrap: WordWrapField{width: 0, explicit: true},
		Commands: []Command{
			{
				Name:  "run",
				Help:  "Run it",
				Flags: []Flag{{Long: "--mode", Arg: "m", Help: longHelp}},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, longHelp) {
		t.Fatal("word_wrap=0 should emit help string raw, but it was changed")
	}
}

// TestWordWrapFieldParsing: word_wrap accepts int, bool, and string forms.
func TestWordWrapFieldParsing(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want int
	}{
		{"unset defaults to 80", "name: x\n", 80},
		{"int", "name: x\nword_wrap: 100\n", 100},
		{"int zero disables", "name: x\nword_wrap: 0\n", 0},
		{"false disables", "name: x\nword_wrap: false\n", 0},
		{"true uses default", "name: x\nword_wrap: true\n", 80},
		{"quoted int", "name: x\nword_wrap: \"60\"\n", 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cfg ShellyCfg
			if err := parseYAML([]byte(c.yaml), &cfg); err != nil {
				t.Fatalf("parseYAML: %v", err)
			}
			if got := cfg.wordWrap(); got != c.want {
				t.Fatalf("wordWrap() = %d, want %d", got, c.want)
			}
		})
	}
}

// ─── help_header_override tests ───────────────────────────────────────────────

// TestHelpHeaderOverrideCommand: when HelpHeaderOverride is set on a command,
// it replaces the default "appname cmdname - help" line in the usage function.
func TestHelpHeaderOverrideCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name:               "deploy",
				Help:               "Deploy the application",
				HelpHeaderOverride: "mytool deploy -- fast deployment tool v2",
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// custom header must appear
	if !strings.Contains(s, `echo "mytool deploy -- fast deployment tool v2"`) {
		t.Fatal("custom help_header_override not found in usage function")
	}
	// default header must NOT appear
	if strings.Contains(s, `echo "mytool deploy - Deploy the application"`) {
		t.Fatal("default header still present despite help_header_override")
	}
}

// TestHelpHeaderOverrideRoot: when HelpHeaderOverride is set on the root cfg
// (no subcommands), it replaces the default "appname - help" line.
func TestHelpHeaderOverrideRoot(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:               "mytool",
		Version:            "0.1.0",
		Help:               "my little tool",
		HelpHeaderOverride: "mytool -- the ultimate tool",
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	if !strings.Contains(s, `echo "mytool -- the ultimate tool"`) {
		t.Fatal("custom help_header_override not found in root usage function")
	}
	if strings.Contains(s, `echo "mytool - my little tool"`) {
		t.Fatal("default root header still present despite help_header_override")
	}
}

// ─── show_examples_on_error tests ─────────────────────────────────────────────

// TestShowExamplesOnErrorArg: when ShowExamplesOnError is true, the required-arg
// error block emits the command's examples before exit 1.
func TestShowExamplesOnErrorArg(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "deploy",
				Help: "Deploy the application",
				Args: []Arg{
					{Name: "env", Help: "Target environment", Required: true},
				},
				Examples:            []string{"mytool deploy production", "mytool deploy staging"},
				ShowExamplesOnError: true,
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find the arg parser function
	start := strings.Index(s, "deploy_parse()")
	if start == -1 {
		t.Fatal("missing deploy_parse()")
	}
	parserEnd := strings.Index(s[start:], "\n}")
	parserBody := s[start : start+parserEnd]

	// examples must appear inside the parser (the error path)
	if !strings.Contains(parserBody, "mytool deploy production") {
		t.Fatal("example not found in required-arg error path")
	}
	if !strings.Contains(parserBody, "mytool deploy staging") {
		t.Fatal("second example not found in required-arg error path")
	}
}

// TestShowExamplesOnErrorArgDisabled: when ShowExamplesOnError is false (default),
// examples do NOT appear inside the arg parser error block.
func TestShowExamplesOnErrorArgDisabled(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "deploy",
				Help: "Deploy",
				Args: []Arg{
					{Name: "env", Help: "Target environment", Required: true},
				},
				Examples: []string{"mytool deploy production"},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	start := strings.Index(s, "deploy_parse()")
	if start == -1 {
		t.Fatal("missing deploy_parse()")
	}
	parserEnd := strings.Index(s[start:], "\n}")
	parserBody := s[start : start+parserEnd]

	// example must NOT appear in the parser body
	if strings.Contains(parserBody, "mytool deploy production") {
		t.Fatal("example leaked into arg parser despite ShowExamplesOnError=false")
	}
}

// TestShowExamplesOnErrorFlag: when ShowExamplesOnError is true, the required-flag
// error block emits examples before exit 1.
func TestShowExamplesOnErrorFlag(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "build",
				Help: "Build the project",
				Flags: []Flag{
					{Long: "--output", Arg: "path", Help: "Output path", Required: true},
				},
				Examples:            []string{"mytool build --output dist/", "mytool build --output /tmp/out"},
				ShowExamplesOnError: true,
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find the flag parser function
	start := strings.Index(s, "build_parse()")
	if start == -1 {
		t.Fatal("missing build_parse()")
	}
	parserEnd := strings.Index(s[start:], "\n}")
	parserBody := s[start : start+parserEnd]

	if !strings.Contains(parserBody, "mytool build --output dist/") {
		t.Fatal("example not found in required-flag error path")
	}
	if !strings.Contains(parserBody, "mytool build --output /tmp/out") {
		t.Fatal("second example not found in required-flag error path")
	}
}

// TestShowExamplesOnErrorFlagDisabled: when ShowExamplesOnError is false,
// examples do NOT appear inside the flag parser error block.
func TestShowExamplesOnErrorFlagDisabled(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "build",
				Help: "Build",
				Flags: []Flag{
					{Long: "--output", Arg: "path", Help: "Output path", Required: true},
				},
				Examples: []string{"mytool build --output dist/"},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	start := strings.Index(s, "build_parse()")
	if start == -1 {
		t.Fatal("missing build_parse()")
	}
	parserEnd := strings.Index(s[start:], "\n}")
	parserBody := s[start : start+parserEnd]

	if strings.Contains(parserBody, "mytool build --output dist/") {
		t.Fatal("example leaked into flag parser despite ShowExamplesOnError=false")
	}
}

// ─── enable_view_markers tests ────────────────────────────────────────────────

// TestDisableViewMarkers: when DisableViewMarkers is true, no "# :command.*"
// section markers appear in the generated script.
func TestDisableViewMarkers(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:               "mytool",
		Version:            "0.1.0",
		DisableViewMarkers: true,
		Commands: []Command{
			{Name: "run", Help: "Run it"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	if strings.Contains(s, "# :command.") {
		t.Fatal("view markers present despite DisableViewMarkers=true")
	}
}

// TestEnableViewMarkersDefault: when DisableViewMarkers is false (default),
// "# :command.*" section markers are present in the generated script.
func TestEnableViewMarkersDefault(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", Help: "Run it"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	if !strings.Contains(s, "# :command.") {
		t.Fatal("view markers missing despite DisableViewMarkers=false (default)")
	}
}

// TestFormatterNone: when formatter is empty or "none", shellGen succeeds without
// attempting to run any formatter binary.
func TestFormatterNone(t *testing.T) {
	for _, fmtr := range []string{"", "none"} {
		t.Run("formatter="+fmtr, func(t *testing.T) {
			dir := t.TempDir()
			oldwd, _ := os.Getwd()
			defer os.Chdir(oldwd)
			os.Chdir(dir)
			os.MkdirAll("src", 0o755)

			cfg := &ShellyCfg{
				Name:      "mytool",
				Version:   "0.1.0",
				Formatter: fmtr,
			}
			if err := cfg.shellGen(); err != nil {
				t.Fatalf("shellGen with formatter=%q: %v", fmtr, err)
			}
			// script must exist and be executable
			info, err := os.Stat("./mytool")
			if err != nil {
				t.Fatalf("output script missing: %v", err)
			}
			if info.Mode()&0o111 == 0 {
				t.Fatal("output script is not executable")
			}
		})
	}
}

// TestFormatterShfmt: when formatter is "shfmt", shellGen either runs shfmt
// (if in PATH) or emits a warning and continues without error.
func TestFormatterShfmt(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:      "mytool",
		Version:   "0.1.0",
		Formatter: "shfmt",
	}
	// shellGen must succeed regardless of whether shfmt is installed
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen with formatter=shfmt: %v", err)
	}
	// script must still exist
	if _, err := os.Stat("./mytool"); err != nil {
		t.Fatalf("output script missing after shfmt formatter: %v", err)
	}
}

// TestFormatterCustomMissing: a custom formatter command that is not in PATH
// returns an error.
func TestFormatterCustomMissing(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:      "mytool",
		Version:   "0.1.0",
		Formatter: "definitely-not-a-real-formatter-xyz",
	}
	err := cfg.shellGen()
	if err == nil {
		t.Fatal("expected error for missing custom formatter, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestFormatterInternal: the internal formatter squashes runs of blank lines
// to a single blank line and leaves normal content untouched.
func TestFormatterInternal(t *testing.T) {
	in := "a\n\n\n\nb\n\n   \n\nc\n"
	got := internalFormat(in)
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("internalFormat left multiple blank lines: %q", got)
	}
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(got, want) {
			t.Fatalf("internalFormat dropped %q: %q", want, got)
		}
	}
}

// TestFormatterInternalRuntime: formatter: internal is applied during shellGen.
func TestFormatterInternalRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{Name: "mytool", Version: "0.1.0", Formatter: "internal"}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen with formatter=internal: %v", err)
	}
	b, err := os.ReadFile("./mytool")
	if err != nil {
		t.Fatalf("read generated script: %v", err)
	}
	if strings.Contains(string(b), "\n\n\n") {
		t.Fatal("generated script still contains multiple consecutive blank lines")
	}
}

// TestFormatterCustomStdinStdout: a custom formatter command is run with the
// script on stdin and its stdout is written back.
func TestFormatterCustomStdinStdout(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:      "mytool",
		Version:   "0.1.0",
		Formatter: "sed s/0.1.0/9.9.9/",
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen with custom formatter: %v", err)
	}
	b, err := os.ReadFile("./mytool")
	if err != nil {
		t.Fatalf("read generated script: %v", err)
	}
	if !strings.Contains(string(b), "9.9.9") {
		t.Fatal("custom formatter output was not written back to the script")
	}
}

// TestSplitCommand covers the small shellwords splitter used for custom
// formatter commands.
func TestSplitCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"shfmt --case-indent --indent 2", []string{"shfmt", "--case-indent", "--indent", "2"}},
		{"sed 's/a b/c d/'", []string{"sed", "s/a b/c d/"}},
		{`cmd "quoted arg"`, []string{"cmd", "quoted arg"}},
		{"  lone  ", []string{"lone"}},
		{"", nil},
	}
	for _, c := range cases {
		got := splitCommand(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("splitCommand(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("splitCommand(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

// TestCatchAllRequired: catch_all with Required:true errors when no extra args provided.
func TestCatchAllRequired(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "run",
				Help: "Run something",
				CatchAll: &CatchAllConfig{
					Label:    "targets",
					Required: true,
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// find the run_parse function body
	start := strings.Index(s, "run_parse()")
	if start == -1 {
		t.Fatal("missing run_parse()")
	}
	end := strings.Index(s[start:], "\nrun_command()")
	if end == -1 {
		end = len(s) - start
	}
	body := s[start : start+end]

	// must trim leading space from other_args
	if !strings.Contains(body, `other_args="${other_args# }"`) {
		t.Fatal("missing other_args trim")
	}
	// must check other_args is non-empty and error with label
	if !strings.Contains(body, `[ -z "${other_args}" ]`) {
		t.Fatal("missing required other_args check")
	}
	if !strings.Contains(body, "missing required argument: TARGETS...") {
		t.Fatal("missing required error message with label")
	}
	// must call usage on error
	if !strings.Contains(body, "usage: mytool run") {
		t.Fatal("missing one-line usage in required catch_all error path")
	}
}

// TestCatchAllRequiredDefaultLabel: catch_all Required:true with no label uses "extra".
func TestCatchAllRequiredDefaultLabel(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{Name: "run", CatchAll: &CatchAllConfig{Required: true}},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")
	if !strings.Contains(s, "missing required argument: EXTRA...") {
		t.Fatal("missing default label 'extra' in required catch_all error message")
	}
}

// TestEnvVarDefault: environment variable with Default emits := assignment in initialize().
func TestEnvVarDefault(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		EnvironmentVariables: []EnvironmentVariable{
			{Name: "LOG_LEVEL", Default: "info", Help: "Log level"},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// default injection must appear somewhere in the script
	if !strings.Contains(s, `: "${LOG_LEVEL:=info}"`) {
		t.Fatal("missing default injection for LOG_LEVEL")
	}
}

// TestEnvVarDefaultPerCommand: per-command env var default injected in command body.
func TestEnvVarDefaultPerCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:    "mytool",
		Version: "0.1.0",
		Commands: []Command{
			{
				Name: "deploy",
				EnvironmentVariables: []EnvironmentVariable{
					{Name: "DEPLOY_ENV", Default: "staging", Help: "Target environment"},
				},
			},
		},
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "mytool")

	// default injection must appear inside deploy_command
	start := strings.Index(s, "deploy_command()")
	if start == -1 {
		t.Fatal("missing deploy_command()")
	}
	body := s[start:]
	if !strings.Contains(body, `: "${DEPLOY_ENV:=staging}"`) {
		t.Fatal("missing default injection for DEPLOY_ENV in deploy_command")
	}
}

// ─── Item 5: DisableHeaderComment ────────────────────────────────────────────

func TestDisableHeaderComment(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", DisableHeaderComment: true}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "generated by shelly") {
		t.Fatal("expected no header comment when DisableHeaderComment=true")
	}
	if !strings.HasPrefix(out, "#!/bin/sh\n") {
		t.Fatalf("expected shebang-only first line, got: %q", out[:40])
	}
}

func TestHeaderCommentPresent(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0"}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "generated by shelly") {
		t.Fatal("expected header comment by default")
	}
}

// ─── Item 7: function_names ───────────────────────────────────────────────────

func TestFunctionNamesRun(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", FunctionNames: FunctionNames{Run: "dispatch", Initialize: "setup"}}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "dispatch() {") {
		t.Fatal("expected renamed run function 'dispatch()'")
	}
	if !strings.Contains(out, "setup() {") {
		t.Fatal("expected renamed initialize function 'setup()'")
	}
	if !strings.Contains(out, "setup\ndispatch") {
		t.Fatalf("expected start section to call setup then dispatch")
	}
	if strings.Contains(out, "\nrun() {") {
		t.Fatal("expected no default 'run()' function when renamed")
	}
}

// ─── Item 8: var_aliases / other_args ────────────────────────────────────────

func TestVarAliasesOtherArgs(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		VarAliases: VarAliases{OtherArgs: "extra_args"},
		Commands: []Command{
			{Name: "do", Help: "do stuff", CatchAll: &CatchAllConfig{Label: "args"}},
		},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "extra_args=") {
		t.Fatal("expected 'extra_args' variable from var_aliases")
	}
	if strings.Contains(out, "other_args=") {
		t.Fatal("expected no default 'other_args' when renamed")
	}
}

// ─── Item 4: tab_indent ───────────────────────────────────────────────────────

func TestTabIndent(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	os.WriteFile("src/root_command.sh", []byte("echo hello\n"), 0o644)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", TabIndent: true}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	// lib body lines should be tab-indented
	if !strings.Contains(out, "\techo hello") {
		t.Fatal("expected tab-indented body line")
	}
}

// TestTabIndentUniform verifies that tab_indent applies to the whole generated
// script (generator boilerplate AND user bodies).
func TestTabIndentUniform(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	// Body with a pre-indented nested line (the case that previously stayed
	// space-indented while the surrounding generated code used tabs).
	os.WriteFile("src/root_command.sh", []byte("echo hello\nif true; then\n  echo nested\nfi\n"), 0o644)

	tabsCfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", TabIndent: true}
	tabbed, err := tabsCfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString (tabs): %v", err)
	}
	spacesCfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", TabIndent: false}
	spaced, err := spacesCfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString (spaces): %v", err)
	}

	// No generated line may begin with two spaces when tab_indent is on.
	for _, line := range strings.Split(tabbed, "\n") {
		if strings.HasPrefix(line, "  ") {
			t.Fatalf("tab-indented output has space-indented line: %q", line)
		}
	}
	// At least some lines must actually be tab-indented.
	if !strings.Contains(tabbed, "\n\t") {
		t.Fatal("expected tab-indented lines in output")
	}
	// Tab output must be exactly expand_tabs of the 2-space output.
	if got := expandTabs(spaced); got != tabbed {
		t.Fatal("tab output is not expandTabs of the space-indented output")
	}
}

// ─── Item 1/2: production mode ───────────────────────────────────────────────

func TestProductionModeNoInspectArgs(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", Env: "production"}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "inspect_args()") {
		t.Fatal("expected no inspect_args in production mode")
	}
}

func TestProductionModeNoMarkers(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", Env: "production"}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "# :command.") {
		t.Fatal("expected no view markers in production mode")
	}
}

func TestDevModeInspectArgs(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0"}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "inspect_args()") {
		t.Fatal("expected inspect_args in dev mode")
	}
}

// ─── Item 2: inspect_args dev utility with vars ───────────────────────────────

func TestInspectArgsIncludesVars(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		Commands: []Command{
			{Name: "deploy", Help: "deploy", Flags: []Flag{{Long: "--env", Arg: "env"}}, Args: []Arg{{Name: "target"}}},
		},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "env") || !strings.Contains(out, "target") {
		t.Fatal("expected inspect_args to reference flag/arg var names")
	}
}

// ─── Item 6: usage_colors ────────────────────────────────────────────────────

func TestAnsiCode(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"red", "\\033[31m"},
		{"green", "\\033[32m"},
		{"blue", "\\033[34m"},
		{"bold", "\\033[1m"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ansiCode(tt.name)
		if got != tt.want {
			t.Errorf("ansiCode(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestUsageColorsCaption(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		UsageColors: UsageColors{Caption: "bold"},
		Commands: []Command{
			{Name: "run", Help: "run stuff"},
		},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "\\033[1m") {
		t.Fatal("expected ANSI bold code in usage output")
	}
}

func TestAnsiCodeColorFunctionNames(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"black", "\\033[30m"},
		{"white", "\\033[37m"},
		{"underlined", "\\033[4m"},
		{"bold_underlined", "\\033[1;4m"},
		{"red_bold", "\\033[1;31m"},
		{"green_underlined", "\\033[4;32m"},
		{"cyan_bold", "\\033[1;36m"},
		{"white_underlined", "\\033[4;37m"},
		{"dim", "\\033[2m"},
		{"GREEN_UNDERLINED", "\\033[4;32m"}, // case-insensitive
		{`\033[9m`, `\033[9m`},              // raw sequence passthrough
		{"notacolor", "notacolor"},          // unknown passthrough
	}
	for _, tt := range tests {
		got := ansiCode(tt.name)
		if got != tt.want {
			t.Errorf("ansiCode(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestUsageColorsColorFunctionNames(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		UsageColors: UsageColors{Caption: "green_underlined"},
		Commands:    []Command{{Name: "run", Help: "run stuff"}},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, "\\033[4;32m") {
		t.Fatal("expected green_underlined ANSI code in usage output")
	}
	// Back-compat: a plain ANSI name still resolves.
	cfg2 := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		UsageColors: UsageColors{Caption: "bold"},
		Commands:    []Command{{Name: "run", Help: "run stuff"}},
	}
	out2, err := cfg2.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out2, "\\033[1m") {
		t.Fatal("expected bold ANSI code in usage output")
	}
}

// ─── Item 9: help multiline ───────────────────────────────────────────────────

func TestHelpMultilineRoot(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{Name: "myapp", Version: "1.0.0", Help: "first line\nsecond line\nthird line"}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if !strings.Contains(out, `myapp - first line`) {
		t.Fatal("expected first help line as summary")
	}
	// bashly layout: multi-line help is shown in full (indented) only in long
	// usage; the short form shows just the first line.
	if !strings.Contains(out, `if [ -n "$long_usage" ]`) {
		t.Fatal("expected multi-line help to gate on long_usage")
	}
	if !strings.Contains(out, `"  second line"`) {
		t.Fatal("expected second help line as a separate indented echo")
	}
}

func TestHelpMultilineCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	cfg := &ShellyCfg{
		Name: "myapp", Version: "1.0.0",
		Commands: []Command{
			{Name: "deploy", Help: "deploy stuff\nMore details here"},
		},
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	// command listing should show only first line
	if !strings.Contains(out, "deploy stuff") {
		t.Fatal("expected first help line in command listing")
	}
	// usage function should have extra echo for second line (long usage form)
	if !strings.Contains(out, `"  More details here"`) {
		t.Fatal("expected second help line as separate echo in command usage")
	}
}

// ─── Item 3: add --upgrade support ───────────────────────────────────────────

func TestAddAddonUpgrade(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)

	// first write — should succeed
	if err := addAddon("colors", false); err != nil {
		t.Fatalf("first addAddon: %v", err)
	}
	// second write without upgrade — should fail
	if err := addAddon("colors", false); err == nil {
		t.Fatal("expected error on second addAddon without upgrade")
	}
	// third write with upgrade — should succeed
	if err := addAddon("colors", true); err != nil {
		t.Fatalf("addAddon with upgrade: %v", err)
	}
}

// ─── Item 6: dependency shorthand syntaxes ────────────────────────────────────

func TestDependencyShorthandSyntaxes(t *testing.T) {
	t.Run("sequence of names", func(t *testing.T) {
		var cfg ShellyCfg
		if err := parseYAML([]byte("name: myapp\ndependencies: [curl, git]\n"), &cfg); err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		if len(cfg.Dependencies) != 2 ||
			cfg.Dependencies[0].Name != "curl" || cfg.Dependencies[1].Name != "git" {
			t.Fatalf("got %+v", cfg.Dependencies)
		}
	})

	t.Run("mapping name to help", func(t *testing.T) {
		var cfg ShellyCfg
		if err := parseYAML([]byte("name: myapp\ndependencies:\n  curl: HTTP client\n"), &cfg); err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		if len(cfg.Dependencies) != 1 ||
			cfg.Dependencies[0].Name != "curl" || cfg.Dependencies[0].Help != "HTTP client" {
			t.Fatalf("got %+v", cfg.Dependencies)
		}
	})

	t.Run("mapping name to object with OR-list", func(t *testing.T) {
		yaml := "name: myapp\ndependencies:\n  http_client:\n    command: [curl, wget]\n    help: HTTP client\n"
		var cfg ShellyCfg
		if err := parseYAML([]byte(yaml), &cfg); err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		d := cfg.Dependencies
		if len(d) != 1 || d[0].Name != "http_client" ||
			strings.Join(d[0].Commands, ",") != "curl,wget" || d[0].Help != "HTTP client" {
			t.Fatalf("got %+v", d)
		}
	})

	t.Run("sequence of object form still works", func(t *testing.T) {
		yaml := "name: myapp\ndependencies:\n  - name: curl\n    version: \"7\"\n"
		var cfg ShellyCfg
		if err := parseYAML([]byte(yaml), &cfg); err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		if len(cfg.Dependencies) != 1 ||
			cfg.Dependencies[0].Name != "curl" || cfg.Dependencies[0].Version != "7" {
			t.Fatalf("got %+v", cfg.Dependencies)
		}
	})

	t.Run("generated deps check covers names", func(t *testing.T) {
		out := (&ShellyCfg{}).generateDepsCheck([]Dependency{{Name: "curl"}, {Name: "git"}})
		if !strings.Contains(out, "command -v curl") || !strings.Contains(out, "command -v git") {
			t.Fatalf("deps check missing names:\n%s", out)
		}
	})
}

// --- issue 0007: input normalization parity ---

// TestCompactShortFlagsRuntime verifies combined short flags (-abc), a trailing
// value flag inside a group, and conjoined -o=value parsing.
func TestCompactShortFlagsRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
      - long: --verbose
        short: -v
      - long: --output
        short: -o
        arg: file
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh",
		[]byte(`echo "source=[$source] force=[$force] verbose=[$verbose] output=[$output]"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	run := func(args ...string) (string, error) {
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		return string(out), err
	}

	out, err := run("download", "-fv", "x")
	if err != nil || !strings.Contains(out, "force=[1]") || !strings.Contains(out, "verbose=[1]") {
		t.Fatalf("-fv: err=%v out=%s", err, out)
	}
	out, err = run("download", "-vf", "x")
	if err != nil || !strings.Contains(out, "force=[1]") || !strings.Contains(out, "verbose=[1]") {
		t.Fatalf("-vf: err=%v out=%s", err, out)
	}
	out, err = run("download", "-fo", "out.txt", "x")
	if err != nil || !strings.Contains(out, "force=[1]") || !strings.Contains(out, "output=[out.txt]") {
		t.Fatalf("-fo: err=%v out=%s", err, out)
	}
	out, err = run("download", "-o=out.txt", "x")
	if err != nil || !strings.Contains(out, "output=[out.txt]") {
		t.Fatalf("-o=out.txt: err=%v out=%s", err, out)
	}
	if _, err := run("download", "-abc", "x"); err == nil {
		t.Fatalf("expected unknown cluster -abc to fail")
	}
}

// TestArgumentNormalizationSettings verifies compact_short_flags and
// conjoined_flag_args can disable the new behavior.
func TestArgumentNormalizationSettings(t *testing.T) {
	setup := func(t *testing.T, extra string) {
		dir := t.TempDir()
		oldwd, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(oldwd) })
		os.Chdir(dir)
		os.MkdirAll("src", 0o755)
		yaml := "name: myapp\nversion: 1.0.0\n" + extra + `
commands:
  - name: download
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
      - long: --verbose
        short: -v
      - long: --output
        short: -o
        arg: file
`
		var cfg ShellyCfg
		if err := parseYAML([]byte(yaml), &cfg); err != nil {
			t.Fatalf("parseYAML: %v", err)
		}
		os.WriteFile("src/download_command.sh",
			[]byte(`echo "force=[$force] verbose=[$verbose] output=[$output]"`+"\n"), 0o644)
		if err := cfg.shellGen(); err != nil {
			t.Fatalf("shellGen: %v", err)
		}
	}
	run := func(args ...string) (string, error) {
		out, err := exec.Command("sh", append([]string{"./myapp"}, args...)...).CombinedOutput()
		return string(out), err
	}

	t.Run("compact disabled rejects combined flags", func(t *testing.T) {
		setup(t, "compact_short_flags: false\n")
		if _, err := run("download", "-fv", "x"); err == nil {
			t.Fatalf("expected -fv to fail when compact_short_flags=false")
		}
	})

	t.Run("conjoined disabled rejects -o=value but keeps -o value", func(t *testing.T) {
		setup(t, "conjoined_flag_args: false\n")
		if _, err := run("download", "-o=out.txt", "x"); err == nil {
			t.Fatalf("expected -o=out.txt to fail when conjoined_flag_args=false")
		}
		out, err := run("download", "-o", "out.txt", "x")
		if err != nil || !strings.Contains(out, "output=[out.txt]") {
			t.Fatalf("-o out.txt: err=%v out=%s", err, out)
		}
	})
}

// TestNoEvalInGeneratedScript guards the "no eval of user input" criterion.
func TestNoEvalInGeneratedScript(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    args:
      - name: source
        required: true
    flags:
      - long: --output
        short: -o
        arg: file
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh", []byte("echo hi\n"), 0o644)
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "eval ") {
		t.Fatalf("generated script must not contain eval:\n%s", out)
	}
}

// runGenerated runs the generated script at relPath with args and returns output.
func runGenerated(t *testing.T, relPath string, args ...string) string {
	t.Helper()
	out, err := exec.Command("sh", append([]string{relPath}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v: %v\n%s", relPath, args, err, out)
	}
	return string(out)
}

// TestSourceDirRuntime: source_dir relocates the src tree.
func TestSourceDirRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("mysrc", 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `
name: myapp
source_dir: mysrc
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("mysrc/download_command.sh", []byte("echo \"ran download\"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	if got := runGenerated(t, "./myapp", "download"); !strings.Contains(got, "ran download") {
		t.Fatalf("unexpected output: %q", got)
	}
}

// TestTargetDirRuntime: target_dir relocates the generated script.
func TestTargetDirRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	yaml := `
name: myapp
target_dir: bin
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/download_command.sh", []byte("echo \"ran download\"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	if _, err := os.Stat("bin/myapp"); err != nil {
		t.Fatalf("expected bin/myapp to exist: %v", err)
	}
	if got := runGenerated(t, "./bin/myapp", "download"); !strings.Contains(got, "ran download") {
		t.Fatalf("unexpected output: %q", got)
	}
}

// TestLibDirRuntime: lib_dir (relative to source_dir) is where library files live.
func TestLibDirRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("mysrc/mylibs", 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `
name: myapp
source_dir: mysrc
lib_dir: mylibs
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("mysrc/mylibs/greet.sh", []byte("echo \"hello $1\"\n"), 0o644)
	os.WriteFile("mysrc/download_command.sh", []byte("greet world\necho done\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	got := runGenerated(t, "./myapp", "download")
	if !strings.Contains(got, "hello world") || !strings.Contains(got, "done") {
		t.Fatalf("lib_dir not searched: %q", got)
	}
}

// TestExtraLibDirsRuntime: extra_lib_dirs are searched in addition to lib_dir.
func TestExtraLibDirsRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)
	os.MkdirAll("extra", 0o755)
	yaml := `
name: myapp
extra_lib_dirs: [extra]
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/lib/localfn.sh", []byte("echo \"local\"\n"), 0o644)
	os.WriteFile("extra/extra_fn.sh", []byte("echo \"extra here\"\n"), 0o644)
	os.WriteFile("src/download_command.sh", []byte("localfn\nextra_fn\necho done\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	got := runGenerated(t, "./myapp", "download")
	if !strings.Contains(got, "local") || !strings.Contains(got, "extra here") || !strings.Contains(got, "done") {
		t.Fatalf("extra_lib_dirs not searched: %q", got)
	}
}

// TestCommandsDirRuntime: commands_dir places each body in a per-command subfolder.
func TestCommandsDirRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("src/commands/download", 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `
name: myapp
commands_dir: commands
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/commands/download/download_command.sh", []byte("echo \"ran download\"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	if got := runGenerated(t, "./myapp", "download"); !strings.Contains(got, "ran download") {
		t.Fatalf("commands_dir not read: %q", got)
	}
}

// TestCommandsDirSkeleton: shellGenCommands scaffolds the body in the nested path.
func TestCommandsDirSkeleton(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	yaml := `
name: myapp
commands_dir: commands
commands:
  - name: download
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	cfg.shellGenCommands()
	if _, err := os.Stat("src/commands/download/download_command.sh"); err != nil {
		t.Fatalf("expected nested skeleton: %v", err)
	}
}

// TestPathSettingsParity: defaults preserve today's layout and resolvers behave.
func TestPathSettingsParity(t *testing.T) {
	var cfg ShellyCfg
	cfg.Name = "myapp"
	if got := cfg.sourceDir(); got != "src" {
		t.Fatalf("default sourceDir = %q, want src", got)
	}
	if got := cfg.targetDir(); got != "." {
		t.Fatalf("default targetDir = %q, want .", got)
	}
	if got := cfg.libDir(); got != filepath.Join("src", "lib") {
		t.Fatalf("default libDir = %q", got)
	}
	if got := cfg.commandPath("download"); got != filepath.Join("src", "download_command.sh") {
		t.Fatalf("default commandPath = %q", got)
	}
	if got := cfg.outputPath(); got != "myapp" {
		t.Fatalf("default outputPath = %q", got)
	}
}

// TestExtraLibDirsParsing: comma scalar, sequence, and "~" (none).
func TestExtraLibDirsParsing(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string
	}{
		{"sequence", "name: myapp\nextra_lib_dirs: [a, b]\n", []string{"a", "b"}},
		{"comma scalar", "name: myapp\nextra_lib_dirs: a, b\n", []string{"a", "b"}},
		{"none tilde", "name: myapp\nextra_lib_dirs: \"~\"\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ShellyCfg
			if err := parseYAML([]byte(tc.yaml), &cfg); err != nil {
				t.Fatalf("parseYAML: %v", err)
			}
			if len(cfg.ExtraLibDirs) != len(tc.want) {
				t.Fatalf("ExtraLibDirs = %v, want %v", cfg.ExtraLibDirs, tc.want)
			}
			for i, w := range tc.want {
				if cfg.ExtraLibDirs[i] != w {
					t.Fatalf("ExtraLibDirs = %v, want %v", cfg.ExtraLibDirs, tc.want)
				}
			}
		})
	}
}

// TestStrictFieldParsing: StrictField accepts bools, "", and custom strings.
func TestStrictFieldParsing(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"true", "name: myapp\nstrict: true\n", "set -euo pipefail\nIFS=$'\\n\\t'"},
		{"false", "name: myapp\nstrict: false\n", "set -e"},
		{"empty string", "name: myapp\nstrict: \"\"\n", ""},
		{"custom", "name: myapp\nstrict: \"set -eux\"\n", "set -eux"},
		{"absent", "name: myapp\n", "set -e"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ShellyCfg
			if err := parseYAML([]byte(tc.yaml), &cfg); err != nil {
				t.Fatalf("parseYAML: %v", err)
			}
			if got := cfg.Strict.Directive(); got != tc.want {
				t.Fatalf("Directive() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStrictModes: generated initialize() emits the right set directive per mode.
func TestStrictModes(t *testing.T) {
	cases := []struct {
		name     string
		strict   StrictField
		wantLine string
		noLine   string
	}{
		{"default false", StrictField{}, "set -e", "set -euo pipefail"},
		{"true", StrictField{boolVal: true}, "set -euo pipefail", ""},
		{"empty none", StrictField{isString: true, strVal: ""}, "", "set -e"},
		{"custom", StrictField{isString: true, strVal: "set -eux"}, "set -eux", "set -euo pipefail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			oldwd, _ := os.Getwd()
			defer os.Chdir(oldwd)
			os.Chdir(dir)
			os.MkdirAll("src", 0o755)
			os.WriteFile("src/build_command.sh", []byte("echo build\n"), 0o644)

			cfg := &ShellyCfg{
				Name:     "mytool",
				Version:  "0.1.0",
				Strict:   tc.strict,
				Commands: []Command{{Name: "build", Help: "Build it"}},
			}
			if err := cfg.shellGen(); err != nil {
				t.Fatalf("shellGen: %v", err)
			}
			s := readGenerated(t, "mytool")
			if tc.wantLine != "" && !strings.Contains(s, tc.wantLine) {
				t.Fatalf("expected %q in output", tc.wantLine)
			}
			if tc.noLine != "" {
				for _, line := range strings.Split(s, "\n") {
					if strings.TrimSpace(line) == tc.noLine || strings.HasPrefix(strings.TrimSpace(line), tc.noLine+" ") {
						t.Fatalf("did not expect %q, found line %q", tc.noLine, line)
					}
				}
			}
		})
	}
}

// TestExpandMessage covers placeholder substitution and shell escaping.
func TestExpandMessage(t *testing.T) {
	if got, want := expandMessage(`Error: %{name} must be one of: %{allowed}`,
		map[string]string{"name": "--x", "allowed": "a, b"}),
		`Error: --x must be one of: a, b`; got != want {
		t.Fatalf("gen-time: got %q want %q", got, want)
	}
	if got, want := expandMessage("Unknown flag: %s", nil, "$1"), "Unknown flag: $1"; got != want {
		t.Fatalf("runtime: got %q want %q", got, want)
	}
	if got, want := expandMessage(`say "hi" $USER`, nil), `say \"hi\" \$USER`; got != want {
		t.Fatalf("escape: got %q want %q", got, want)
	}
	if got, want := expandMessage("a %s b %s", nil, "$1", "$2"), "a $1 b $2"; got != want {
		t.Fatalf("two runtime: got %q want %q", got, want)
	}
}

// TestMessageDefaultsGenerated checks the built-in messages are emitted.
func TestMessageDefaultsGenerated(t *testing.T) {
	s := (&ShellyCfg{Name: "mytool"}).generateParser(Command{Name: "mytool"}, true, "mytool_usage", "mytool")
	if !strings.Contains(s, `invalid option: $1`) {
		t.Fatalf("default invalid_flag missing:\n%s", s)
	}
}

// TestStringsOverrideRuntime checks src/shelly-strings.yml overrides messages in
// the generated script.
func TestStringsOverrideRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: download
    help: Download a file
    args:
      - name: source
        required: true
    flags:
      - long: --force
        short: -f
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	stringsYAML := `
invalid_flag: "bad option: %s"
missing_required_argument: "need <%{name}>"
`
	if err := os.WriteFile("src/shelly-strings.yml", []byte(stringsYAML), 0o644); err != nil {
		t.Fatalf("write strings: %v", err)
	}
	if err := os.WriteFile("src/download_command.sh",
		[]byte("echo \"source=[$source]\"\n"), 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	out, _ := exec.Command("sh", "./myapp", "download", "--nope", "x").CombinedOutput()
	if !strings.Contains(string(out), "bad option: --nope") {
		t.Fatalf("custom invalid_flag not used:\n%s", out)
	}
	out2, err := exec.Command("sh", "./myapp", "download").CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing-arg failure, got success:\n%s", out2)
	}
	if !strings.Contains(string(out2), "need <SOURCE>") {
		t.Fatalf("custom missing_required_argument not used:\n%s", out2)
	}
}

// TestAddStringsAddon checks `shelly add strings` scaffolds a loadable file.
func TestAddStringsAddon(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := addAddon("strings", false); err != nil {
		t.Fatalf("addAddon: %v", err)
	}
	data, err := os.ReadFile("src/shelly-strings.yml")
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}
	if !strings.Contains(string(data), "invalid_flag:") ||
		!strings.Contains(string(data), "missing_required_argument:") {
		t.Fatalf("scaffold missing keys:\n%s", data)
	}
	// scaffold must be loadable and every default key must round-trip
	var cfg ShellyCfg
	if err := cfg.loadStrings(); err != nil {
		t.Fatalf("loadStrings: %v", err)
	}
	if len(cfg.messages) != len(defaultMessages) {
		t.Fatalf("scaffold loaded %d messages, want %d", len(cfg.messages), len(defaultMessages))
	}
	for k, v := range defaultMessages {
		if cfg.messages[k] != v {
			t.Fatalf("scaffold key %q = %q, want %q", k, cfg.messages[k], v)
		}
	}
	// second add without upgrade must fail
	if err := addAddon("strings", false); err == nil {
		t.Fatalf("expected error adding strings twice without upgrade")
	}
}

// ─── Item 14: yaml/config/ini addon libraries ────────────────────────────────

func TestAddYamlIniConfigAddons(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	// deliberately do NOT pre-create src/lib: add must create it

	if err := addAddon("yaml", false); err != nil {
		t.Fatalf("addAddon yaml: %v", err)
	}
	if b, err := os.ReadFile("src/lib/yaml.sh"); err != nil || !strings.Contains(string(b), "yaml_load()") {
		t.Fatalf("yaml.sh missing/invalid: %v", err)
	}
	if err := addAddon("ini", false); err != nil {
		t.Fatalf("addAddon ini: %v", err)
	}
	if b, err := os.ReadFile("src/lib/ini.sh"); err != nil || !strings.Contains(string(b), "ini_load()") {
		t.Fatalf("ini.sh missing/invalid: %v", err)
	}
	if err := addAddon("config", true); err != nil {
		t.Fatalf("addAddon config: %v", err)
	}
	if b, err := os.ReadFile("src/lib/config.sh"); err != nil || !strings.Contains(string(b), "config_get()") {
		t.Fatalf("config.sh missing/invalid: %v", err)
	}
	// config also refreshes ini.sh
	if _, err := os.Stat("src/lib/ini.sh"); err != nil {
		t.Fatalf("config addon should also write ini.sh: %v", err)
	}
	// second add without upgrade fails, with upgrade succeeds
	if err := addAddon("yaml", false); err == nil {
		t.Fatal("expected error adding yaml twice without upgrade")
	}
	if err := addAddon("yaml", true); err != nil {
		t.Fatalf("addAddon yaml upgrade: %v", err)
	}
}

// TestVerbatimLibFunctions: a lib file defining functions is emitted as-is,
// while a body-only lib file is wrapped as <stem>() { ... }.
func TestVerbatimLibFunctions(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src/lib", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile("src/lib/mylib.sh", []byte("foo() {\n  echo foo\n}\n\nbar() {\n  foo\n}\n"), 0o644)
	os.WriteFile("src/lib/greet.sh", []byte("echo \"hello $1\"\n"), 0o644)

	cfg := &ShellyCfg{Name: "testapp", Version: "0.1.0"}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	s := readGenerated(t, "testapp")
	if !strings.Contains(s, "foo() {") || !strings.Contains(s, "bar() {") {
		t.Fatalf("verbatim functions missing\n%s", s)
	}
	if strings.Contains(s, "mylib() {") {
		t.Fatalf("verbatim lib should not be wrapped as mylib()")
	}
	if !strings.Contains(s, "greet() {") {
		t.Fatalf("body-only lib should be wrapped as greet()")
	}
}

// TestVerbatimLibDuplicateDetection: duplicate function names across lib units
// are fatal even when one comes from a verbatim (multi-function) file.
func TestVerbatimLibDuplicateDetection(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src/lib", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile("src/lib/a.sh", []byte("dupe() {\n  echo a\n}\n"), 0o644)
	os.WriteFile("src/lib/dupe.sh", []byte("echo b\n"), 0o644)

	cfg := &ShellyCfg{Name: "testapp", Version: "0.1.0"}
	if err := cfg.shellGen(); err == nil {
		t.Fatal("expected duplicate function error")
	}
}

// TestIniLibraryRuntime: the embedded ini library works in a generated script.
func TestIniLibraryRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll("src/lib", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile("src/lib/ini.sh", []byte(iniContent), 0o644)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: show
    help: Show config
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/show_command.sh",
		[]byte("ini_load config.ini\necho \"name=$(ini_get name) user=$(ini_get login.user) dflt=$(ini_get nope.x fallback)\"\n"), 0o644)
	os.WriteFile("config.ini", []byte("name = Alice\n\n[login]\nuser = bob\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	out, err := exec.Command("sh", "./myapp", "show").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"name=Alice", "user=bob", "dflt=fallback"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func writeLibrarySourceFixture(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("libsrc/scripts/util", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := `
hello:
  help: A greeting library
  post_install_message: Add hello to your before hook
  files:
    - source: scripts/hello.sh
      target: '%{user_source_dir}/lib/hello.sh'
    - source: scripts/util/extra.sh
      target: '%{user_lib_dir}/extra_%{user_ext}.sh'
skip:
  help: Works without a source dir
  skip_src_check: true
  files:
    - source: scripts/hello.sh
      target: '%{user_lib_dir}/hello.sh'
`
	if err := os.WriteFile("libsrc/libraries.yml", []byte(yaml), 0o644); err != nil {
		t.Fatalf("write libraries.yml: %v", err)
	}
	os.WriteFile("libsrc/scripts/hello.sh", []byte("# [@shelly-upgrade]\nhello() { echo hi; }\n"), 0o644)
	os.WriteFile("libsrc/scripts/util/extra.sh", []byte("extra() { echo extra; }\n"), 0o644)
}

// TestLibrarySourceInstall: a libraries.yml source installs files to resolved
// %{user_*} targets and substitutes the upgrade marker.
func TestLibrarySourceInstall(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	writeLibrarySourceFixture(t)

	if err := addFromSource("libsrc", "hello", false, false); err != nil {
		t.Fatalf("addFromSource: %v", err)
	}
	got, err := os.ReadFile("src/lib/hello.sh")
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !strings.Contains(string(got), "[@shelly-upgrade libsrc;hello]") {
		t.Fatalf("marker not substituted: %q", got)
	}
	if _, err := os.Stat("src/lib/extra_sh.sh"); err != nil {
		t.Fatalf("extra target not resolved: %v", err)
	}

	// Existing files are skipped without --upgrade.
	os.WriteFile("src/lib/hello.sh", []byte("local change\n"), 0o644)
	if err := addFromSource("libsrc", "hello", false, false); err != nil {
		t.Fatalf("second add: %v", err)
	}
	got, _ = os.ReadFile("src/lib/hello.sh")
	if !strings.Contains(string(got), "local change") {
		t.Fatalf("file overwritten without upgrade: %q", got)
	}

	// --upgrade overwrites.
	if err := addFromSource("libsrc", "hello", true, false); err != nil {
		t.Fatalf("upgrade add: %v", err)
	}
	got, _ = os.ReadFile("src/lib/hello.sh")
	if !strings.Contains(string(got), "hello() {") {
		t.Fatalf("upgrade did not overwrite: %q", got)
	}
}

// TestLibrarySourceSkipSrcCheck: skip_src_check bypasses the source-dir guard.
func TestLibrarySourceSkipSrcCheck(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	writeLibrarySourceFixture(t) // intentionally no src/ created

	// A library without skip_src_check errors when src/ is missing.
	if err := addFromSource("libsrc", "hello", false, false); err == nil {
		t.Fatalf("expected error when src dir missing and skip_src_check false")
	}
	// skip_src_check bypasses the source-dir guard.
	if err := addFromSource("libsrc", "skip", false, false); err != nil {
		t.Fatalf("skip_src_check install failed: %v", err)
	}
	if _, err := os.Stat("src/lib/hello.sh"); err != nil {
		t.Fatalf("file not installed: %v", err)
	}
}

// TestLibrarySourceUnknown: an unknown library is a clear error.
func TestLibrarySourceUnknown(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	writeLibrarySourceFixture(t)

	err := addFromSource("libsrc", "nope", false, false)
	if err == nil || !strings.Contains(err.Error(), "unknown library") {
		t.Fatalf("want unknown-library error, got %v", err)
	}
	if err := listLibraries("libsrc"); err != nil {
		t.Fatalf("listLibraries: %v", err)
	}
	if _, err := loadLibrarySource("does-not-exist"); err == nil {
		t.Fatalf("expected error for a missing source directory")
	}
}

// TestResolveLibraryTarget: %{user_*} tokens expand to the config defaults.
func TestResolveLibraryTarget(t *testing.T) {
	cfg := &ShellyCfg{}
	cases := map[string]string{
		"%{user_source_dir}/lib/x.sh":   "src/lib/x.sh",
		"%{user_target_dir}/x":          "./x",
		"%{user_lib_dir}/y.%{user_ext}": "src/lib/y.sh",
	}
	for in, want := range cases {
		if got := resolveLibraryTarget(in, cfg); got != want {
			t.Fatalf("resolveLibraryTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAddonUpgradeMarkers: every built-in addon writes the upgrade marker.
func TestAddonUpgradeMarkers(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	assertMarker := func(path, name string) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		want := "[@shelly-upgrade " + name + "]"
		if !strings.Contains(string(b), want) {
			t.Fatalf("%s missing marker %q: %q", path, want, b)
		}
	}

	order := []struct{ name, path string }{
		{"validations", "src/lib/validations.sh"},
		{"colors", "src/lib/colors.sh"},
		{"yaml", "src/lib/yaml.sh"},
		{"ini", "src/lib/ini.sh"},
		{"hooks", "src/before.sh"},
		{"preamble", "src/preamble.sh"},
		{"strings", "src/shelly-strings.yml"},
	}
	for _, tc := range order {
		if err := addAddon(tc.name, false); err != nil {
			t.Fatalf("addAddon %s: %v", tc.name, err)
		}
		assertMarker(tc.path, tc.name)
	}
	// config installs config.sh (and refreshes ini.sh); check the config marker.
	if err := addAddon("config", true); err != nil {
		t.Fatalf("addAddon config: %v", err)
	}
	assertMarker("src/lib/config.sh", "config")
}

// TestGenerateUpgradeBuiltin: upgradeLibs refreshes a marked built-in file.
func TestGenerateUpgradeBuiltin(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	if err := addAddon("colors", false); err != nil {
		t.Fatalf("addAddon: %v", err)
	}
	// Corrupt the file while keeping the marker.
	os.WriteFile("src/lib/colors.sh", []byte("## [@shelly-upgrade colors]\ncorrupted\n"), 0o644)

	if err := upgradeLibs(&ShellyCfg{}); err != nil {
		t.Fatalf("upgradeLibs: %v", err)
	}
	b, err := os.ReadFile("src/lib/colors.sh")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(b), "red()") {
		t.Fatalf("builtin not restored: %q", b)
	}
	if !strings.Contains(string(b), "[@shelly-upgrade colors]") {
		t.Fatalf("marker lost after upgrade: %q", b)
	}
}

// TestGenerateUpgradeCustomSource: upgradeLibs refreshes a marked file installed
// from a custom libraries source.
func TestGenerateUpgradeCustomSource(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	writeLibrarySourceFixture(t)

	if err := addFromSource("libsrc", "hello", false, false); err != nil {
		t.Fatalf("addFromSource: %v", err)
	}
	// Corrupt the file while keeping the fully-qualified marker.
	os.WriteFile("src/lib/hello.sh", []byte("# [@shelly-upgrade libsrc;hello]\ncorrupted\n"), 0o644)

	if err := upgradeLibs(&ShellyCfg{}); err != nil {
		t.Fatalf("upgradeLibs: %v", err)
	}
	b, err := os.ReadFile("src/lib/hello.sh")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(b), "hello() {") {
		t.Fatalf("custom source not restored: %q", b)
	}
}

// TestGenerateUpgradeUnknownLibraryWarns: an unknown library is a warning, not
// a failure, and its file is left untouched.
func TestGenerateUpgradeUnknownLibraryWarns(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)
	os.WriteFile("src/lib/custom.sh", []byte("## [@shelly-upgrade nosuchlib]\necho hi\n"), 0o644)

	if err := upgradeLibs(&ShellyCfg{}); err != nil {
		t.Fatalf("upgradeLibs should not fail on unknown library: %v", err)
	}
	b, _ := os.ReadFile("src/lib/custom.sh")
	if !strings.Contains(string(b), "echo hi") {
		t.Fatalf("unknown-library file was modified: %q", b)
	}
}

// TestUpgradeMarkerStrippedFromOutput: the upgrade marker lives in the source
// files but must not appear in the generated script.
func TestUpgradeMarkerStrippedFromOutput(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	var cfg ShellyCfg
	if err := parseYAML([]byte("name: myapp\ncommands:\n  - name: greet\n    help: hi\n"), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/greet_command.sh", []byte("echo hi\n"), 0o644)
	if err := addAddon("colors", false); err != nil {
		t.Fatalf("addAddon: %v", err)
	}

	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "[@shelly-upgrade") {
		t.Fatalf("upgrade marker leaked into generated script")
	}
	if !strings.Contains(out, "red()") {
		t.Fatalf("colors library missing from generated script")
	}
}

// TestStripDesiredCommentsUnit exercises the "##" line filter directly.
func TestStripDesiredCommentsUnit(t *testing.T) {
	cases := []struct{ in, want string }{
		{"## note\n", ""},
		{"  ## indented\n", ""},
		{"\t## tab-indented\n", ""},
		{"## last line no newline", ""},
		{"### triple\n", ""},
		{"# keep\n", "# keep\n"},
		{"#!/bin/sh\n", "#!/bin/sh\n"},
		{"echo hi\n## gone\necho bye\n", "echo hi\necho bye\n"},
		{"echo '# not a comment'\n", "echo '# not a comment'\n"},
	}
	for _, c := range cases {
		if got := stripDesiredComments(c.in); got != c.want {
			t.Errorf("stripDesiredComments(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStripDesiredCommentsGenerated: "##" comments are dropped from command
// bodies, lib files, and overridable partials, while ordinary "#" comments and
// content survive.
func TestStripDesiredCommentsGenerated(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src/lib", 0o755)

	var cfg ShellyCfg
	if err := parseYAML([]byte("name: myapp\ncommands:\n  - name: build\n    help: build it\n"), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/build_command.sh",
		[]byte("echo build\n## desired-command note\n# visible note\n"), 0o644)
	os.WriteFile("src/lib/greet.sh",
		[]byte("## lib build note\ngreet() {\n  echo hello\n}\n"), 0o644)
	os.WriteFile("src/before.sh",
		[]byte("## partial build note\n:\n"), 0o644)

	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	for _, leaked := range []string{"desired-command note", "lib build note", "partial build note"} {
		if strings.Contains(out, leaked) {
			t.Errorf("desired comment %q leaked into generated script", leaked)
		}
	}
	if !strings.Contains(out, "# visible note") {
		t.Errorf("ordinary # comment was stripped from generated script")
	}
	if !strings.Contains(out, "echo build") || !strings.Contains(out, "echo hello") {
		t.Errorf("body/lib content missing from generated script")
	}
}

func TestConfigImportMerge(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile("src/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("shelly.yml", `name: cli
version: 0.1.0
commands:
  - import: download.yml
  - import: upload.sh
`)
	write("common_flags.yml", `- long: --force
  short: -f
  help: Overwrite existing files
`)
	write("download.yml", `name: download
alias: d
help: Download a file
args:
  - name: source
    required: true
flags:
  import: common_flags.yml
`)
	write("upload.sh", `# front matter
name: upload
alias: u
help: Upload a file
args:
  - name: source
    required: true
flags:
  import: common_flags.yml
---
echo "uploading $source"
`)

	cfg, err := readConfigE()
	if err != nil {
		t.Fatalf("readConfigE: %v", err)
	}
	if cfg == nil {
		t.Fatal("readConfigE returned nil config")
	}
	if len(cfg.Commands) != 2 {
		t.Fatalf("expected 2 imported commands, got %d", len(cfg.Commands))
	}
	for _, c := range cfg.Commands {
		if len(c.Args) != 1 || c.Args[0].Name != "source" || !c.Args[0].Required {
			t.Errorf("command %q: unexpected args: %+v", c.Name, c.Args)
		}
		if len(c.Flags) != 1 || c.Flags[0].Long != "--force" || c.Flags[0].Short != "-f" {
			t.Errorf("command %q: nested flags import not merged: %+v", c.Name, c.Flags)
		}
	}
	if cfg.Commands[0].Name != "download" || cfg.Commands[1].Name != "upload" {
		t.Errorf("unexpected command order/names: %q, %q", cfg.Commands[0].Name, cfg.Commands[1].Name)
	}
}

func TestConfigImportErrors(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		files   map[string]string
		wantSub string
	}{
		{
			name:    "missing import file",
			config:  "name: cli\ncommands:\n  - import: nope.yml\n",
			wantSub: "cannot find import file",
		},
		{
			name:    "sh without front matter",
			config:  "name: cli\ncommands:\n  - import: plain.sh\n",
			files:   map[string]string{"plain.sh": "echo hi\n"},
			wantSub: "front matter",
		},
		{
			name:    "invalid yaml",
			config:  "name: cli\ncommands:\n  - import: bad.yml\n",
			files:   map[string]string{"bad.yml": "name: x\n  bad: : :\n"},
			wantSub: "cannot parse import file",
		},
		{
			name:    "non-mapping root",
			config:  "name: cli\ncommands:\n  - import: scalar.yml\n",
			files:   map[string]string{"scalar.yml": "just a scalar\n"},
			wantSub: "cannot find a valid YAML",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			oldwd, _ := os.Getwd()
			defer os.Chdir(oldwd)
			os.Chdir(dir)
			if err := os.MkdirAll("src", 0o755); err != nil {
				t.Fatal(err)
			}
			os.WriteFile("src/shelly.yml", []byte(tc.config), 0o644)
			for name, content := range tc.files {
				os.WriteFile("src/"+name, []byte(content), 0o644)
			}
			_, err := readConfigE()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestConfigImportDuplicateCommand(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile("src/shelly.yml", []byte("name: cli\ncommands:\n  - import: a.yml\n  - import: b.yml\n"), 0o644)
	os.WriteFile("src/a.yml", []byte("name: dup\nhelp: A\n"), 0o644)
	os.WriteFile("src/b.yml", []byte("name: dup\nhelp: B\n"), 0o644)

	cfg, err := readConfigE()
	if err != nil {
		t.Fatalf("readConfigE: %v", err)
	}
	errs := cfg.validateSemantics()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "duplicate command name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate command name error, got %v", errs)
	}
}

func TestConfigImportRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	if err := os.MkdirAll("src", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile("src/shelly.yml", []byte("name: cli\ncommands:\n  - import: download.yml\n"), 0o644)
	os.WriteFile("src/download.yml", []byte(`name: download
help: Download a file
args:
  - name: source
    required: true
flags:
  import: common_flags.yml
`), 0o644)
	os.WriteFile("src/common_flags.yml", []byte("- long: --force\n  short: -f\n"), 0o644)
	os.WriteFile("src/download_command.sh", []byte("echo \"downloading $source force=$force\"\n"), 0o644)

	cfg, err := readConfigE()
	if err != nil {
		t.Fatalf("readConfigE: %v", err)
	}
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	out, err := exec.Command("sh", "./cli", "download", "--force", "http://x").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got := string(out); !strings.Contains(got, "downloading http://x force=1") {
		t.Errorf("unexpected output: %q", got)
	}
}

// TestDepsVarName covers the stable dependency-variable naming rule.
func TestDepsVarName(t *testing.T) {
	cases := map[string]string{
		"curl":        "curl",
		"http_client": "http_client",
		"http-client": "http_client",
		"node.js":     "node_js",
		"a b":         "a_b",
	}
	for in, want := range cases {
		if got := depsVarName(in); got != want {
			t.Errorf("depsVarName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDepsGetHelperRuntime checks deps_get works in POSIX sh for a single dep
// and returns 1 (printing nothing) for an unknown name.
func TestDepsGetHelperRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: run
    help: Run
    dependencies:
      - name: sh
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/run_command.sh", []byte(`p="$(deps_get sh)"
echo "sh=$p"
if deps_get nope; then echo "found"; else echo "missing"; fi`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	out, err := exec.Command("sh", "./myapp", "run").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "sh=/") {
		t.Errorf("deps_get sh did not resolve a path: %q", got)
	}
	if !strings.Contains(got, "missing") {
		t.Errorf("deps_get for unknown name should return 1: %q", got)
	}
}

// TestDepsGetORListRuntime checks deps_get resolves an OR-list to whichever
// candidate command is actually available.
func TestDepsGetORListRuntime(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: run
    help: Run
    dependencies:
      http_client:
        command: [definitely_missing_xyz, sh]
        help: HTTP client
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	os.WriteFile("src/run_command.sh", []byte(`echo "client=$(deps_get http_client)"
echo "sh=$(deps_get sh)"
echo "raw=$deps_sh"`+"\n"), 0o644)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}
	out, err := exec.Command("sh", "./myapp", "run").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "client=/") {
		t.Errorf("deps_get http_client did not resolve via OR-list: %q", got)
	}
	if !strings.Contains(got, "sh=/") {
		t.Errorf("deps_get sh (candidate alias) did not resolve: %q", got)
	}
	if !strings.Contains(got, "raw=/") {
		t.Errorf("deps_<name> scalar missing: %q", got)
	}
}

// TestNoDepsGetWithoutDeps ensures the helper is only emitted when needed.
func TestNoDepsGetWithoutDeps(t *testing.T) {
	yaml := `
name: myapp
version: 1.0.0
commands:
  - name: run
    help: Run
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	if strings.Contains(out, "deps_get()") {
		t.Errorf("deps_get should not be emitted when there are no dependencies")
	}
}

// TestInspectArgsNestedCommands ensures the generated inspect_args() collects
// flags/args/catch_all from nested commands, not just top-level ones, and reads
// them with ${var:-} so it is safe under set -u.
func TestInspectArgsNestedCommands(t *testing.T) {
	yaml := `
name: gup
version: 1.0.0
commands:
  - name: install
    help: Install things
    commands:
      - name: go
        help: Install Go
        args:
          - name: goversion
            required: true
        flags:
          - long: --force
            short: -f
        catch_all:
          label: extras
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	out, err := cfg.shellGenToString()
	if err != nil {
		t.Fatalf("shellGenToString: %v", err)
	}
	for _, want := range []string{
		"inspect_args() {",
		"_inspect_collect_install_go() {",
		`'goversion' "$goversion"`,
		`'--force' "$force"`,
		`if [ -n "${other_args}" ]; then _inspect_other_section`,
		"    install_go)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inspect_args missing %q", want)
		}
	}
	if !strings.Contains(out, "${goversion+x}") {
		t.Errorf("inspect_args should detect set vars with ${var+x} for set -u safety")
	}
}

// TestInspectArgsRuntime verifies at runtime that inspect_args reflects the
// command that actually ran: a command with no flags/args prints "args: none",
// even when another command in the same tool declares an argument. Regression
// test for the static-inspector bug where every command leaked every var.
func TestInspectArgsRuntime(t *testing.T) {
	yaml := `
name: mytool
version: 1.0.0
commands:
  - name: alpha
    help: Alpha
  - name: beta
    help: Beta
    args:
      - name: font
`
	var cfg ShellyCfg
	if err := parseYAML([]byte(yaml), &cfg); err != nil {
		t.Fatalf("parseYAML: %v", err)
	}
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)
	if err := cfg.shellGen(); err != nil {
		t.Fatalf("shellGen: %v", err)
	}

	out, err := exec.Command("sh", "./mytool", "alpha").CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "args: none") {
		t.Fatalf("expected 'args: none' for command with no args:\n%s", out)
	}
	if strings.Contains(string(out), "font:") || strings.Contains(string(out), "'font'") {
		t.Fatalf("command alpha must not report another command's arg:\n%s", out)
	}

	// beta declares the `font` arg; running it should report that arg.
	out, err = exec.Command("sh", "./mytool", "beta", "Iosevka").CombinedOutput()
	if err != nil {
		t.Fatalf("run beta: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "- ${args[font]} = Iosevka") {
		t.Fatalf("expected font arg reported for beta:\n%s", out)
	}
}
