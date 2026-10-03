package service

import (
	"errors"
	"reflect"
	"testing"

	kardianos "github.com/kardianos/service"
)

type fakeServiceManager struct {
	status        kardianos.Status
	statusErr     error
	installErr    error
	startErr      error
	installCalled int
	startCalled   int
	stopCalled    int
}

func (f *fakeServiceManager) Status() (kardianos.Status, error) { return f.status, f.statusErr }
func (f *fakeServiceManager) Install() error                    { f.installCalled++; return f.installErr }
func (f *fakeServiceManager) Start() error                      { f.startCalled++; return f.startErr }
func (f *fakeServiceManager) Stop() error                       { f.stopCalled++; return nil }

func TestEnsureRunningRestartsRunningService(t *testing.T) {
	prg := &fakeServiceManager{status: kardianos.StatusRunning}
	if err := ensureRunning(prg); err != nil {
		t.Fatal(err)
	}
	if prg.installCalled != 0 || prg.startCalled != 1 || prg.stopCalled != 1 {
		t.Fatalf("运行中的服务应先停止再启动，install=%d start=%d stop=%d", prg.installCalled, prg.startCalled, prg.stopCalled)
	}
}

func TestEnsureRunningStartsStoppedServiceWithoutReinstall(t *testing.T) {
	prg := &fakeServiceManager{status: kardianos.StatusStopped}
	if err := ensureRunning(prg); err != nil {
		t.Fatal(err)
	}
	if prg.installCalled != 0 {
		t.Fatalf("已注册服务不应重复安装，install=%d", prg.installCalled)
	}
	if prg.startCalled != 1 {
		t.Fatalf("已停止服务应启动一次，start=%d", prg.startCalled)
	}
}

func TestEnsureRunningInstallsWhenNotInstalled(t *testing.T) {
	prg := &fakeServiceManager{statusErr: errors.New("service not found")}
	if err := ensureRunning(prg); err != nil {
		t.Fatal(err)
	}
	if prg.installCalled != 1 || prg.startCalled != 1 {
		t.Fatalf("未安装服务应先安装再启动，install=%d start=%d", prg.installCalled, prg.startCalled)
	}
}

func TestEnsureRunningReportsInstallFailure(t *testing.T) {
	prg := &fakeServiceManager{statusErr: errors.New("not installed"), installErr: errors.New("需要管理员权限")}
	if err := ensureRunning(prg); err == nil {
		t.Fatal("expected install failure to be reported")
	}
	if prg.startCalled != 0 {
		t.Fatalf("安装失败时不应启动，start=%d", prg.startCalled)
	}
}

func TestIsServiceMissingErrorRecognizesNotInstalled(t *testing.T) {
	if !isServiceMissingError(errors.New("the service is not installed")) {
		t.Fatal("expected not-installed service error to be ignored")
	}
}

func TestRegisteredExecutableMatches(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
		want     bool
	}{
		{name: "systemd installed copy", output: "{ path=/opt/certd/.certd-client-service ; argv[]=/opt/certd/.certd-client-service service run }", expected: "/opt/certd/.certd-client-service", want: true},
		{name: "legacy main executable", output: "{ path=/opt/certd/certd-client ; argv[]=/opt/certd/certd-client service run }", expected: "/opt/certd/.certd-client-service", want: false},
		{name: "windows installed copy", output: "BINARY_PATH_NAME : C:\\certd\\.certd-client-service.exe service run", expected: "C:\\certd\\.certd-client-service.exe", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := executableInServiceOutput(test.output, test.expected); got != test.want {
				t.Fatalf("executableInServiceOutput() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestClassifyIgnoresNonServiceCommands(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"tui"}, {"start"}, {"sync"}} {
		if _, ok := Classify(args); ok {
			t.Fatalf("expected non-service command %#v to be ignored", args)
		}
	}
}

func TestClassifyRecognizesServiceActions(t *testing.T) {
	cases := map[string]string{
		"install":   "install",
		"uninstall": "uninstall",
		"start":     "start",
		"stop":      "stop",
		"run":       "run",
		"ensure":    "ensure",
		"RUN":       "run",
	}
	for arg, want := range cases {
		action, ok := Classify([]string{"service", arg})
		if !ok || action != want {
			t.Fatalf("service %q => (%q,%v) want (%q,true)", arg, action, ok, want)
		}
	}
}

func TestClassifyUnknownOrMissingActionIsHelp(t *testing.T) {
	if action, ok := Classify([]string{"service"}); !ok || action != "help" {
		t.Fatalf("expected help for missing action, got (%q,%v)", action, ok)
	}
	if action, ok := Classify([]string{"service", "bogus"}); !ok || action != "help" {
		t.Fatalf("expected help for unknown action, got (%q,%v)", action, ok)
	}
}

func TestBuildConfigCarriesServiceIdentityAndRunArgs(t *testing.T) {
	cfg := buildConfig("/opt/certd-client/certd-client", "/opt/certd-client")
	if cfg.Name != Name {
		t.Fatalf("unexpected service name %q", cfg.Name)
	}
	if cfg.DisplayName == "" || cfg.Description == "" {
		t.Fatalf("expected display name and description, got %#v", cfg)
	}
	if cfg.WorkingDirectory != "/opt/certd-client" {
		t.Fatalf("expected working directory, got %q", cfg.WorkingDirectory)
	}
	if !reflect.DeepEqual(cfg.Arguments, []string{"service", "run"}) {
		t.Fatalf("expected run arguments, got %#v", cfg.Arguments)
	}
}
