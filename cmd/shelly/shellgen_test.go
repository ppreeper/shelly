package main

import (
	"os"
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

	if !strings.Contains(s, "upload_parse_flags()") {
		t.Fatal("missing upload_parse_flags()")
	}
	if !strings.Contains(s, "upload_command()") {
		t.Fatal("missing upload_command()")
	}
	idx := strings.Index(s, "upload_command()")
	if !strings.Contains(s[idx:], "upload_parse_flags \"$@\"") {
		t.Fatal("upload_command does not invoke upload_parse_flags")
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
	if !strings.Contains(s, "--token is required") {
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
	if !strings.Contains(s, "<target> is required") {
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
	if !strings.Contains(s, "root_parse_flags()") {
		t.Fatal("missing root_parse_flags()")
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
	if !strings.Contains(s, "root_parse_args()") {
		t.Fatal("missing root_parse_args()")
	}
	if !strings.Contains(s, "<source> is required") {
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
	if !strings.Contains(s, "do_deploy_parse_flags()") {
		t.Fatal("missing do_deploy_parse_flags() function")
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

// TestBeforeAfterHooks: src/before.sh and src/after.sh are included in run().
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
	if !strings.Contains(s, "before_hook") {
		t.Fatal("missing before_hook from src/before.sh")
	}
	if !strings.Contains(s, "after_hook") {
		t.Fatal("missing after_hook from src/after.sh")
	}
	// before must appear inside run() (after its declaration), after must also be inside run()
	beforeIdx := strings.Index(s, "before_hook")
	afterIdx := strings.Index(s, "after_hook")
	runIdx := strings.Index(s, "run()")
	if beforeIdx == -1 || afterIdx == -1 {
		t.Fatal("hooks not found")
	}
	if beforeIdx < runIdx {
		t.Fatal("before_hook should appear inside run() body (after run() declaration)")
	}
	if afterIdx < runIdx {
		t.Fatal("after_hook should appear inside run() body (after run() declaration)")
	}
	if beforeIdx > afterIdx {
		t.Fatal("before_hook should appear before after_hook")
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
	if !strings.Contains(out, "#!/usr/bin/env sh") {
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
		Strict:   true,
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
				Argfile: true,
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
	// global flag parser must exist
	if !strings.Contains(s, "mytool_parse_flags") {
		t.Fatal("expected mytool_parse_flags for global flags")
	}
	// global flag parser must be called before command dispatch
	parseIdx := strings.Index(s, "mytool_parse_flags")
	dispatchIdx := strings.Index(s, "build_command")
	if parseIdx > dispatchIdx {
		t.Fatal("global flag parser must be called before command dispatch")
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

	if err := addAddon("validations"); err != nil {
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

	if err := addAddon("colors"); err != nil {
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

	if err := addAddon("hooks"); err != nil {
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
	if err := addAddon("nonexistent"); err == nil {
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
				Argfile: true,
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

	// find the run_parse_flags function
	start := strings.Index(s, "run_parse_flags()")
	if start == -1 {
		t.Fatal("missing run_parse_flags()")
	}
	// find the end of run_parse_flags (next function)
	end := strings.Index(s[start:], "\nrun_parse_args()")
	if end == -1 {
		end = strings.Index(s[start:], "\nrun_command()")
	}
	if end == -1 {
		end = len(s) - start
	}
	flagParserBody := s[start : start+end]

	// When catch_help is true, -h|--help must NOT call usage and exit
	// It should fall into other_args accumulation instead
	if strings.Contains(flagParserBody, "-h|--help) mytool_run_usage") {
		t.Fatal("catch_help:true should NOT intercept -h|--help with usage call; it must fall through to other_args")
	}
	// The -h and --help args should land in other_args
	if !strings.Contains(flagParserBody, "other_args") {
		t.Fatal("run_parse_flags missing other_args (catch_all required for catch_help)")
	}
}

// TestDoubleDashPositionals: args after '--' must be fed into positional
// parsing, not discarded.
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

	// After '--' the remaining args should be passed into parse_args.
	// The command function must call parse_args after parse_flags.
	start := strings.Index(s, "run_command()")
	if start == -1 {
		t.Fatal("missing run_command()")
	}
	cmdBody := s[start:]
	if !strings.Contains(cmdBody, "run_parse_args") {
		t.Fatal("run_command must call run_parse_args")
	}
	// The flag parser '--' case must pass remaining args to the caller via $@.
	// After 'shift; break', $@ contains the post-'--' args.
	// The command wrapper must call parse_args "$@" (not with a fixed $1..$N).
	fpStart := strings.Index(s, "run_parse_flags()")
	fpEnd := strings.Index(s[fpStart:], "\nrun_parse_args()")
	if fpEnd == -1 {
		fpEnd = strings.Index(s[fpStart:], "\nrun_command()")
	}
	if fpEnd == -1 {
		fpEnd = len(s) - fpStart
	}
	flagBody := s[fpStart : fpStart+fpEnd]
	// '--' must shift and break so remaining $@ is preserved
	if !strings.Contains(flagBody, "--) shift; break") {
		t.Fatal("flag parser must handle '--' with 'shift; break'")
	}
	// run_command must call run_parse_args "$@" so post-'--' args reach positionals
	if !strings.Contains(cmdBody[:strings.Index(cmdBody, "}")], `run_parse_args "$@"`) {
		t.Fatal("run_command must call run_parse_args \"$@\" so -- remainder reaches positional parser")
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
		WordWrap: 80,
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

// TestWordWrapDefault: when word_wrap is 0 (default), long help strings are
// emitted raw (no wrapping).
func TestWordWrapDefault(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	longHelp := "This is a very long help description that should not be wrapped because word_wrap is zero"

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
	if !strings.Contains(s, longHelp) {
		t.Fatal("word_wrap=0 should emit help string raw, but it was changed")
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
	start := strings.Index(s, "deploy_parse_args()")
	if start == -1 {
		t.Fatal("missing deploy_parse_args()")
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

	start := strings.Index(s, "deploy_parse_args()")
	if start == -1 {
		t.Fatal("missing deploy_parse_args()")
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
	start := strings.Index(s, "build_parse_flags()")
	if start == -1 {
		t.Fatal("missing build_parse_flags()")
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

	start := strings.Index(s, "build_parse_flags()")
	if start == -1 {
		t.Fatal("missing build_parse_flags()")
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

// TestFormatterUnknown: an unrecognised formatter name returns an error.
func TestFormatterUnknown(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll("src", 0o755)

	cfg := &ShellyCfg{
		Name:      "mytool",
		Version:   "0.1.0",
		Formatter: "prettify",
	}
	err := cfg.shellGen()
	if err == nil {
		t.Fatal("expected error for unknown formatter, got nil")
	}
	if !strings.Contains(err.Error(), "unknown formatter") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
