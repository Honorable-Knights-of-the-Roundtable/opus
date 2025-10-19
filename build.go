//go:build ignore

package main

import (
	"archive/tar"
	"bufio"
	"strings"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"github.com/klauspost/compress/zstd"
)

const (
	opusVersion = "1.5.2"
	opusURL     = "https://downloads.xiph.org/releases/opus/opus-" + opusVersion + ".tar.gz"
	vendorDir   = "deps/opus"

	// MSYS2 MinGW64 opus package - pre-built binaries
	msys2OpusURL = "https://mirror.msys2.org/mingw/mingw64/mingw-w64-x86_64-opus-1.5.2-1-any.pkg.tar.zst"

	// System-wide install location on Windows
	systemInstallDir = "C:\\opus"
)

func main() {
	if err := build(); err != nil {
		fatal("Build failed: %v", err)
	}
	fmt.Println("✓ Build successful!")
}

func build() error {
	if runtime.GOOS == "windows" {
		return buildWindows() 
	}
	
	if runtime.GOOS == "linux" {
		return buildLinux()
	}
	return nil

}
func buildLinux() error {
	fmt.Println("Detecting available audio backends...")

	// Check which backends are available
	if hasBackendLinux("opus") {
		fmt.Printf("  ✓ %s found\n", "opus")
	} else {
		err := handleNoBackendLinux()
		if err != nil {
			fmt.Printf("  Opus failed to install\n %v", err)
			return err
		}
	}
	return nil
}

func buildWindows() error {
	// Check if we need admin privileges (for installation or PATH modification)
	systemLibPath := filepath.Join(systemInstallDir, "lib", "libopus.a")
	needsInstall := !fileExists(systemLibPath)
	binPath := filepath.Join(systemInstallDir, "bin")
	needsPathUpdate := needsInstall || !isInSystemPath(binPath)

	if needsPathUpdate && !isAdmin() {
		fmt.Println("Administrator privileges required for installation and PATH modification.")
		fmt.Println("Requesting elevation...")
		return rerunAsAdmin()
	}

	// Check if already installed system-wide
	if fileExists(systemLibPath) {
		fmt.Printf("✓ libopus already installed at %s\n", systemInstallDir)

		// Even if installed, ensure it's in PATH
		if !isInSystemPath(binPath) {
			fmt.Println("Adding to system PATH...")
			if err := addToSystemPath(binPath); err != nil {
				fmt.Printf("⚠ Warning: Could not add to PATH: %v\n", err)
				fmt.Printf("Please manually add to PATH: %s\n", binPath)
			} else {
				fmt.Println("✓ Added to system PATH")
				fmt.Println("  (Restart your terminal/IDE for PATH changes to take effect)")
			}
		} else {
			fmt.Printf("✓ %s is already in PATH\n", binPath)
		}
		return nil
	}

	// Download and extract
	fmt.Println("Downloading opus from MSYS2...")
	tarPath := filepath.Join(vendorDir, "opus.tar.zst")
	os.MkdirAll(vendorDir, 0755)

	if err := downloadFile(msys2OpusURL, tarPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	fmt.Println("Extracting...")
	extractDir := filepath.Join(vendorDir, "extracted")
	if err := extractTarZst(tarPath, extractDir); err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	// Copy files to system location
	fmt.Printf("Installing to %s (requires admin privileges)...\n", systemInstallDir)

	// The MSYS2 package extracts to mingw64/* structure
	msys2Root := filepath.Join(extractDir, "mingw64")

	if err := copyDir(filepath.Join(msys2Root, "lib"), filepath.Join(systemInstallDir, "lib")); err != nil {
		return fmt.Errorf("failed to copy lib: %w", err)
	}

	if err := copyDir(filepath.Join(msys2Root, "include"), filepath.Join(systemInstallDir, "include")); err != nil {
		return fmt.Errorf("failed to copy include: %w", err)
	}

	if err := copyDir(filepath.Join(msys2Root, "bin"), filepath.Join(systemInstallDir, "bin")); err != nil {
		return fmt.Errorf("failed to copy bin: %w", err)
	}

	fmt.Println("✓ Installation successful!")

	// Add to PATH
	binPath = filepath.Join(systemInstallDir, "bin")
	fmt.Println("Adding to system PATH...")
	if err := addToSystemPath(binPath); err != nil {
		fmt.Printf("⚠ Warning: Could not add to PATH automatically: %v\n", err)
		fmt.Printf("Please manually add to PATH: %s\n", binPath)
	} else {
		fmt.Println("✓ Added to system PATH")
		fmt.Println("\n⚠ IMPORTANT: Restart your terminal/shell for PATH changes to take effect!")
		fmt.Println("   After restarting, you can build your project.")
	}

	fmt.Printf("\nInstallation locations:\n")
	fmt.Printf("  Libraries: %s\n", filepath.Join(systemInstallDir, "lib"))
	fmt.Printf("  Headers: %s\n", filepath.Join(systemInstallDir, "include"))
	fmt.Printf("  Binaries: %s\n", binPath)

	return nil
}

func handleNoBackendLinux() error {
	fmt.Println("\n❌ No audio encoder found!")
	fmt.Println("\nYou need to install one of the following:")

	distro := detectDistro()

	switch distro {
	case "debian", "ubuntu":
		fmt.Println("\n  # Debian/Ubuntu:")
		fmt.Println("  sudo apt-get install opus-tools libopus0 libopus-dev")
	case "fedora", "rhel", "centos":
		fmt.Println("\n  # Fedora/RHEL/CentOS:")
		fmt.Println("  sudo dnf opus-devel opusfile-devel")
	case "arch":
		fmt.Println("\n  # Arch Linux:")
		fmt.Println("  sudo pacman -S opus")
	default:
		fmt.Println("\n  Please install development packages for one of:")
		fmt.Println("    - libopus")
	}

	fmt.Println("\nWould you like to install one now? (y/N)")
	if !askConfirmation() {
		return fmt.Errorf("audio backend required to build")
	}

	return installBackendLinux(distro)
}

func hasBackendLinux(pkgName string) bool {
	cmd := exec.Command("pkg-config", "--exists", pkgName)
	return cmd.Run() == nil
}

// TODO: Allow for installing of backend based on user input choice
func installBackendLinux(distro string) error {
	var cmd *exec.Cmd

	switch distro {
	case "debian", "ubuntu":
		fmt.Println("Running: sudo apt-get install opus-tools libopus0 libopus-dev")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "apt", "install", "-y", "opus-tools", "libopus0", "libopus-dev")
	case "fedora", "rhel", "centos":
		fmt.Println("Running: sudo dnf opus-devel opusfile-devel")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "dnf", "install", "-y", "opus-devel", "opusfile-devel")
	case "arch":
		fmt.Println("Running: sudo pacman -S opus")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", "opus")
	default:
		return fmt.Errorf("automatic installation not supported for your distribution")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Println("\n✓ Installation successful! Retrying build...")
	return buildLinux()
}

func askConfirmation() bool {
	// When running under go generate, stdin is not connected to the terminal.
	// We need to explicitly open /dev/tty to read from the terminal.
	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Println("\nCouldn't open the terminal input, try installing the dependency yourself with the previously mentioned command.")
		// If we can't open the terminal, default to no
		return false
	}
	defer tty.Close()

	reader := bufio.NewReader(tty)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
func detectDistro() string {
	// Check /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}

	content := string(data)
	if strings.Contains(strings.ToLower(content), "ubuntu") {
		return "ubuntu"
	}
	if strings.Contains(strings.ToLower(content), "debian") {
		return "debian"
	}
	if strings.Contains(strings.ToLower(content), "fedora") {
		return "fedora"
	}
	if strings.Contains(strings.ToLower(content), "rhel") || strings.Contains(strings.ToLower(content), "red hat") {
		return "rhel"
	}
	if strings.Contains(strings.ToLower(content), "centos") {
		return "centos"
	}
	if strings.Contains(strings.ToLower(content), "arch") {
		return "arch"
	}

	return "unknown"
}

func extractTarZst(tarPath, dstDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
			return err
	}
	defer file.Close()

	// Decompress zstd
	d, err := zstd.NewReader(file)
	if err != nil {
			return err
	}
	defer d.Close()

	// Extract tar
	tr := tar.NewReader(d)
	for {
			header, err := tr.Next()
			if err == io.EOF {
					break
			}
			if err != nil {
					return err
			}

			target := filepath.Join(dstDir, header.Name)
			switch header.Typeflag {
			case tar.TypeDir:
					os.MkdirAll(target, 0755)
			case tar.TypeReg:
					os.MkdirAll(filepath.Dir(target), 0755)
					f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					if err != nil {
							return err
					}
					io.Copy(f, tr)
					f.Close()
			}
	}
	return nil
}

func extractTarGz(tarPath, dstDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dstDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			io.Copy(f, tr)
			f.Close()
		}
	}
	return nil
}

func Decompress(in io.Reader, out io.Writer) error {
    d, err := zstd.NewReader(in)
    if err != nil {
        return err
    }
    defer d.Close()
    
    // Copy content...
    _, err = io.Copy(out, d)
    return err
}


// func writeCGOFlags() error {
// 	ldflags := fmt.Sprintf("-L${SRCDIR}/deps/opus/lib/%s -lopus", runtime.GOOS)
// 	// Windows doesn't need -lm
// 	if runtime.GOOS != "windows" {
// 		ldflags += " -lm"
// 	}
//
// 	content := fmt.Sprintf(`// Code generated by build.go. DO NOT EDIT.
//
// //go:build static
//
// package opus
//
// /*
// #cgo windows CFLAGS: -I${SRCDIR}/deps/opus/include
// #cgo windows LDFLAGS: %s
// #cgo linux pkg-config: opus
// */
// import "C"
// `, ldflags)
//
// 	if err := os.WriteFile("cgo_flags_static.go", []byte(content), 0644); err != nil {
// 		return err
// 	}
//
// 	fmt.Println("✓ Generated cgo_flags_static.go")
// 	return nil
// }

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}


func copyFile(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}

func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func addToSystemPath(dir string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on Windows")
	}

	// Use PowerShell to add to system PATH permanently
	// This requires admin privileges
	psScript := fmt.Sprintf(`
		$path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
		if ($path -notlike '*%s*') {
			$newPath = $path + ';%s'
			[Environment]::SetEnvironmentVariable('Path', $newPath, 'Machine')
			Write-Output 'Added to PATH'
		} else {
			Write-Output 'Already in PATH'
		}
	`, dir, dir)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}

	return nil
}

func isAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	// Run 'net session' which requires admin privileges
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}

func isInSystemPath(dir string) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	psScript := fmt.Sprintf(`
		$path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
		if ($path -like '*%s*') {
			Write-Output 'true'
		} else {
			Write-Output 'false'
		}
	`, dir)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "true"
}

func rerunAsAdmin() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("only supported on Windows")
	}

	// Get the path to the Go executable and current script
	_, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Use PowerShell Start-Process with -Verb RunAs to elevate
	// We need to re-run "go run build.go"
	cwd, _ := os.Getwd()
	psScript := fmt.Sprintf(`Start-Process -FilePath "go" -ArgumentList "run","build.go" -Verb RunAs -WorkingDirectory "%s" -Wait`, cwd)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to elevate: %w (you may have cancelled the UAC prompt)", err)
	}

	// Exit the current non-elevated process
	fmt.Println("\n✓ Elevated process completed. You can now build your project.")
	os.Exit(0)
	return nil
}
