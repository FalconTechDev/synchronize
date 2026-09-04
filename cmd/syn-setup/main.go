package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

//go:embed syn.exe
var synBinary []byte

//go:embed logo.png
var logoData []byte

var Version = "dev"

func main() {
	uninstall := len(os.Args) > 1 && (os.Args[1] == "/uninstall" || os.Args[1] == "-u")

	banner()

	var err error
	if uninstall {
		err = doUninstall()
	} else {
		err = doInstall()
	}

	if err != nil {
		fmt.Printf("\n  ERROR: %v\n", err)
		pause()
		os.Exit(1)
	}
	pause()
}

func banner() {
	fmt.Println()
	fmt.Println("  ===============================================")
	fmt.Println("     synchronize (syn) " + Version)
	fmt.Println("     automatic git sync across your machines")
	fmt.Println("  ===============================================")
	fmt.Println()
}

func installDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "syn")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".synchronize")
}

func doInstall() error {
	dir := installDir()

	fmt.Printf("  Installing to %s\n\n", dir)

	step("creating folders")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(homeDir(), 0o700); err != nil {
		return err
	}
	ok()

	step("writing syn.exe")
	exePath := filepath.Join(dir, "syn.exe")
	if err := writeReplacing(exePath, synBinary); err != nil {
		return fmt.Errorf("write syn.exe: %w", err)
	}
	ok()

	step("writing logo")
	if err := os.WriteFile(filepath.Join(homeDir(), "logo.png"), logoData, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "uninstall-hint.txt"),
		[]byte("To uninstall, run this installer again with /uninstall\n"), 0o644); err != nil {
		return err
	}
	ok()

	step("adding to PATH")
	added, err := addToPath(dir)
	if err != nil {
		return fmt.Errorf("update PATH: %w", err)
	}
	if added {
		ok()
	} else {
		fmt.Println("already there")
	}

	step("verifying")
	out, err := exec.Command(exePath, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("syn.exe failed to run: %v\n%s", err, out)
	}
	ok()

	fmt.Println()
	fmt.Println("  Installed: " + strings.TrimSpace(string(out)))
	fmt.Println()
	fmt.Println("  Open a NEW terminal, then:")
	fmt.Println()
	fmt.Println("    syn auth login --pat <your-github-token>")
	fmt.Println("    syn repo add owner/repo")
	fmt.Println("    syn watch")
	fmt.Println()
	fmt.Println("  Run this installer with /uninstall to remove.")
	return nil
}

func doUninstall() error {
	dir := installDir()
	fmt.Printf("  Removing %s\n\n", dir)

	step("stopping running instances")
	exec.Command("taskkill", "/F", "/IM", "syn.exe").Run()
	time.Sleep(500 * time.Millisecond)
	ok()

	step("removing files")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	ok()

	step("cleaning PATH")
	if err := removeFromPath(dir); err != nil {
		return err
	}
	ok()

	fmt.Println()
	fmt.Printf("  Done. Your config in %s was kept.\n", homeDir())
	fmt.Println("  Delete that folder manually if you want it gone too.")
	return nil
}

func writeReplacing(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		exec.Command("taskkill", "/F", "/IM", "syn.exe").Run()
		time.Sleep(300 * time.Millisecond)
		os.Rename(path, path+".old")
		defer os.Remove(path + ".old")
	}
	return os.WriteFile(path, data, 0o755)
}

func addToPath(dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	cur, _, err := k.GetStringValue("PATH")
	if err != nil && err != registry.ErrNotExist {
		return false, err
	}
	for _, p := range strings.Split(cur, ";") {
		if strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			return false, nil
		}
	}

	next := dir
	if cur != "" {
		next = strings.TrimRight(cur, ";") + ";" + dir
	}
	if err := k.SetExpandStringValue("PATH", next); err != nil {
		return false, err
	}
	broadcastEnvChange()
	return true, nil
}

func removeFromPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	cur, _, err := k.GetStringValue("PATH")
	if err != nil {
		return nil
	}
	var kept []string
	for _, p := range strings.Split(cur, ";") {
		if p == "" || strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			continue
		}
		kept = append(kept, p)
	}
	if err := k.SetExpandStringValue("PATH", strings.Join(kept, ";")); err != nil {
		return err
	}
	broadcastEnvChange()
	return nil
}

func step(s string) {
	fmt.Printf("  %-28s", s+" ...")
}

func ok() {
	fmt.Println("done")
}

func pause() {
	fmt.Println()
	fmt.Print("  Press Enter to close...")
	fmt.Scanln()
}
