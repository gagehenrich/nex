package main

import (
	"database/sql"
	//"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	//_ "github.com/mattn/go-sqlite3"  
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v3"
)

var (
	RelPath         	= "/var/lib/nex" 
	AutocompleteFile 	string 
	DefaultUsername 	= "username" 
	DefaultPassword 	= "password" 
	SQLCipherKey = "5442f2aa9ef98ca87f997b658b6acf8c1f5d5f2bb2e864d10cc3edd812e9350a" // placeholder
)

type Host struct {
	Site, Hostname, IP, Username, Password string
	RemotePort, SocksPort                  int
}

type NEX struct {
	dbPath string
	db     *sql.DB
}

type YAMLConfig struct {
    All struct {
        Username   string `yaml:"username"`
        Password   string `yaml:"password"`
        SocksPort  int    `yaml:"socks_port"`
        RemotePort int    `yaml:"remote_port"`
    } `yaml:"all"`
    Sites map[string]struct {
        Hosts map[string]struct {
            IPAddr     string `yaml:"ipaddr"`
            Username   string `yaml:"username,omitempty"`
            Password   string `yaml:"password,omitempty"`
            SocksPort  int    `yaml:"socks_port,omitempty"`
            RemotePort int    `yaml:"remote_port,omitempty"`
        } `yaml:"hosts"`
    } `yaml:"sites"`
}

func NewNEX() (*NEX, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	userNEXDir := filepath.Join(homeDir, ".local", "nex")
	AutocompleteFile = filepath.Join(userNEXDir, "autocomplete.sh")
    
	// 1. Create the root-only directory with restrictive permissions (0700)
	if err := os.MkdirAll(RelPath, 0700); err != nil {
		// This will likely fail for a non-root user, enforcing 'sudo' use.
		return nil, fmt.Errorf("failed to create nex directory %s: ensure you run with 'sudo' for the first time or if the directory is missing: %w", RelPath, err)
	}
	
	dbPath := filepath.Join(RelPath, "nex.db3")
	
	b := &NEX{dbPath: dbPath}
	
	// Check if database exists to determine if install/encryption pragma is needed
	isNewDB := false
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		isNewDB = true
	}
	
	// Create the connection string with the SQLCipher key
    // The 'cipher_page_size=4096' and 'kdf_iter=64000' are standard SQLCipher pragmas.
	// NOTE: SQLCipherKey is now a VAR that is dynamically set by the Makefile
	connStr := fmt.Sprintf("file:%s?_pragma_key='x'%s&_pragma_cipher_page_size=4096&_pragma_kdf_iter=64000", 
        dbPath, SQLCipherKey)
        
	// 2. Open database with SQLCipher configuration
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open encrypted database: %w", err)
	}
	b.db = db
	
    // 3. If it's a new DB, run the install/schema creation
	if isNewDB {
		if err := b.install(); err != nil {
			// Try to close and delete the partially created encrypted file on failure
			b.Close() 
			os.Remove(dbPath)
			return nil, err
		}
	}
    
    // Ensure the file permissions are correct after creation (only root can read/write)
    // This is vital as umask might not be 077
    if err := os.Chmod(dbPath, 0600); err != nil {
        return nil, fmt.Errorf("failed to set restrictive permissions on database: %w", err)
    }
	
	return b, nil
}

func (b *NEX) Close() error {
	if b.db != nil {
		return b.db.Close()
	}
	return nil
}

func (b *NEX) logf(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s %s\n", timestamp, fmt.Sprintf(format, args...))
}

// --- INSTALL: Uses the already-opened encrypted connection ---
func (b *NEX) install() error {
	// The DB connection is already open and encrypted via NewNEX
	
	_, err := b.db.Exec(`
CREATE TABLE IF NOT EXISTS configurations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key TEXT UNIQUE NOT NULL,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS hosts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    site TEXT NOT NULL,
    hostname TEXT NOT NULL,
    ipaddr TEXT NOT NULL,
    username TEXT,
    password TEXT,
    remote_port INTEGER,
    socks_port INTEGER,
    UNIQUE(site, hostname)
);`)
	if err != nil {
		return fmt.Errorf("failed to create tables on encrypted DB: %w", err)
	}
	
	b.logf("SQLCipher database created at %s (requires 'sudo' to access)", b.dbPath)
	return nil
}

// --- REST OF THE CODE (No Change, as sshpass usage is retained) ---

func (b *NEX) addHost(h Host) error {
	if h.Username == "" {
		h.Username = DefaultUsername
	}
	if h.Password == "" {
		h.Password = DefaultPassword
	}
	if h.RemotePort == 0 {
		h.RemotePort = 22
	}
	if h.SocksPort == 0 {
		h.SocksPort = 9999
	}
	
	_, err := b.db.Exec(`INSERT INTO hosts (site, hostname, ipaddr, username, password, remote_port, socks_port)
	                     VALUES (?, ?, ?, ?, ?, ?, ?)`,
		h.Site, h.Hostname, h.IP, h.Username, h.Password, h.RemotePort, h.SocksPort)
	if err != nil {
		return fmt.Errorf("failed to add host: %w", err)
	}
	
	b.logf("Added host: %s:%s (%s)", h.Site, h.Hostname, h.IP)
	return nil
}

func (b *NEX) queryHost(site, hostname string) (*Host, error) {
	row := b.db.QueryRow(`SELECT ipaddr, username, password, remote_port, socks_port FROM hosts WHERE site=? AND hostname=?`,
		site, hostname)
	h := &Host{Site: site, Hostname: hostname}
	err := row.Scan(&h.IP, &h.Username, &h.Password, &h.RemotePort, &h.SocksPort)
	if err != nil {
		return nil, fmt.Errorf("host %s:%s not found: %w", site, hostname, err)
	}
	return h, nil
}

func (b *NEX) deleteHost(site, hostname string) error {
	_, err := b.db.Exec(`DELETE FROM hosts WHERE site=? AND hostname=?`, site, hostname)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}
	b.logf("Deleted host: %s:%s", site, hostname)
	return nil
}

func (b *NEX) dumpDB() error {
	rows, err := b.db.Query(`SELECT site, hostname, ipaddr, username, password, remote_port, socks_port FROM hosts ORDER BY site, hostname`)
	if err != nil {
		return err
	}
	defer rows.Close()
	
	fmt.Println("Site | Hostname | IP | Username | Password | RemotePort | SocksPort")
	fmt.Println(strings.Repeat("-", 80))
	
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.Site, &h.Hostname, &h.IP, &h.Username, &h.Password, &h.RemotePort, &h.SocksPort); err != nil {
			return err
		}
		fmt.Printf("%s | %s | %s | %s | %s | %d | %d\n",
			h.Site, h.Hostname, h.IP, h.Username, h.Password, h.RemotePort, h.SocksPort)
	}
	
	return rows.Err()
}

func (b *NEX) formatHost(hostSpec string) (*Host, error) {
	// Check if it's a site:hostname format
	if strings.Contains(hostSpec, ":") && !strings.Contains(hostSpec, ".") {
		parts := strings.SplitN(hostSpec, ":", 2)
		if len(parts) == 2 {
			return b.queryHost(parts[0], parts[1])
		}
	}
	
	// Default to IP with default credentials from Makefile-injected variables
	return &Host{
		IP:         hostSpec,
		Username:   DefaultUsername,
		Password:   DefaultPassword,
		RemotePort: 22,
		SocksPort:  9999,
	}, nil
}

func (b *NEX) sshExec(h *Host, cmd string) error {
	sshCmd := exec.Command("sshpass", "-p", h.Password, "ssh",
		"-p", fmt.Sprintf("%d", h.RemotePort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-y",
		fmt.Sprintf("%s@%s", h.Username, h.IP),
		cmd)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Stdin = os.Stdin
	return sshCmd.Run()
}

func (b *NEX) scpGet(h *Host, remotePath, localPath string) error {
	cmd := exec.Command("sshpass", "-p", h.Password, "scp",
		"-P", fmt.Sprintf("%d", h.RemotePort),
		"-rp",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("%s@%s:%s", h.Username, h.IP, remotePath),
		localPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *NEX) scpPut(h *Host, localPath, remotePath string) error {
	cmd := exec.Command("sshpass", "-p", h.Password, "scp",
		"-P", fmt.Sprintf("%d", h.RemotePort),
		"-rp",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		localPath,
		fmt.Sprintf("%s@%s:%s", h.Username, h.IP, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *NEX) connectHost(hostSpec string, command string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	if command != "" {
		// Check if command needs sudo
		if strings.Contains(command, "sudo") {
			fmt.Print("sudo password: ")
			password, err := terminal.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				return err
			}
			command = fmt.Sprintf("echo %s | sudo -S %s", string(password), strings.TrimPrefix(command, "sudo "))
		}
	}
	
	return b.sshExec(h, command)
}

func (b *NEX) getFile(hostSpec, remotePath string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	filename := filepath.Base(remotePath)
	if err := b.scpGet(h, remotePath, "./"+filename); err != nil {
		return err
	}
	
	b.logf("Downloaded: %s", filename)
	return nil
}

func (b *NEX) putFile(hostSpec, localPath string) error {
    h, err := b.formatHost(hostSpec)
    if err != nil {
        return err
    }

    homeDir, err := nex.execRemoteCommand(h, `echo "$HOME"`)
    if err != nil {
        return fmt.Errorf("failed to determine remote home directory: %v", err)
    }
    homeDir = strings.TrimSpace(homeDir)

    filename := filepath.Base(localPath)
    remotePath := path.Join(homeDir, filename)

    if err := b.scpPut(h, localPath, remotePath); err != nil {
        return fmt.Errorf("scp upload failed: %v", err)
    }

    b.logf("Uploaded %s to %s:%s", localPath, h.IP, remotePath)
    return nil
}


func (b *NEX) pingHost(hostSpec string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	cmd := exec.Command("timeout", "2", "ping", "-c1", h.IP)
	if err := cmd.Run(); err != nil {
		b.logf("Ping failed for %s", h.IP)
		return err
	}
	
	b.logf("Ping successful for %s", h.IP)
	return nil
}

func (b *NEX) socksProxy(hostSpec string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	cmd := exec.Command("sshpass", "-p", h.Password, "ssh",
		"-p", fmt.Sprintf("%d", h.RemotePort),
		"-D", fmt.Sprintf("%d", h.SocksPort),
		"-q", "-C", "-N", "-f",
		fmt.Sprintf("%s@%s", h.Username, h.IP))
	
	if err := cmd.Run(); err != nil {
		return err
	}
	
	b.logf("SOCKS proxy started at 127.0.0.1:%d for %s", h.SocksPort, h.IP)
	return nil
}

func (b *NEX) shareIDs(hostSpec string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	b.logf("Exchanging SSH keys with %s", h.IP)
	
	// ssh-keyscan
	cmd := exec.Command("ssh-keyscan", h.IP)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	
	homeDir, _ := os.UserHomeDir()
	knownHosts := filepath.Join(homeDir, ".ssh", "known_hosts")
	f, err := os.OpenFile(knownHosts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.Write(out)
	
	// ssh-copy-id
	cmd = exec.Command("sshpass", "-p", h.Password, "ssh-copy-id", "-f",
		fmt.Sprintf("-p %d %s@%s", h.RemotePort, h.Username, h.IP))
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (b *NEX) buildDB(yamlFile string) error {
    data, err := os.ReadFile(yamlFile)
    if err != nil {
        return fmt.Errorf("failed to read YAML file: %w", err)
    }

    var config YAMLConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return fmt.Errorf("failed to parse YAML: %w", err)
    }

    count := 0

    // Iterate through sites
    for siteName, site := range config.Sites {
        // Iterate through hosts in each site
        for hostname, hostData := range site.Hosts {
            h := Host{
                Site:     siteName,
                Hostname: hostname,
                IP:       hostData.IPAddr,
            }

            // Use host-specific values if provided, otherwise fall back to defaults from 'all'
            if hostData.Username != "" {
                h.Username = hostData.Username
            } else {
                h.Username = config.All.Username
            }

            if hostData.Password != "" {
                h.Password = hostData.Password
            } else {
                h.Password = config.All.Password
            }

            if hostData.RemotePort != 0 {
                h.RemotePort = hostData.RemotePort
            } else {
                h.RemotePort = config.All.RemotePort
            }

            if hostData.SocksPort != 0 {
                h.SocksPort = hostData.SocksPort
            } else {
                h.SocksPort = config.All.SocksPort
            }

            if err := b.addHost(h); err != nil {
                fmt.Printf("Warning: failed to add %s:%s - %v\n", h.Site, h.Hostname, err)
            } else {
                count++
            }
        }
    }

    b.logf("Successfully imported %d hosts from %s", count, yamlFile)
    return nil
}

func (b *NEX) cloneHost(hostSpec string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	dumpName := fmt.Sprintf("%s.tgz", h.IP)
	
	excludeDirs := []string{
		// Virtual / kernel / runtime FS
		"proc",
		"sys",
		"dev",
		"run",
		"tmp",
		"mnt",
		"media",
		"sys/fs/cgroup",

		// Package managers / cache
		"var/cache",
		"var/lib/yum",
		"var/lib/dnf",
		"var/lib/apt/lists",

		// Logs / monitoring
		"var/log",
		"var/log/messages*",
		"var/log/sa",
		"*log",
		"*bak",

		// Databases (huge + unsafe to copy live)
		"var/lib/mysql",
		"var/lib/pgsql",
		"*.db3",
		"*.mmdb",
		"*.dump",
		"*.sqlite",
		"*.sqlite3",

		// Large stale archives
		"*tar.gz",
		"*tgz",

		// Network/system configs that should NOT be cloned
		"etc/fstab",
		"etc/mtab",
		"etc/udev",
		"etc/NetworkManager",
		"etc/sysconfig/network-scripts",

		// Cluster FS (GPFS / EXA / EAS / similar)
		"EAS",
		"EXA",
		"gpfs",
		"gsfs",
		"usr/lpp/mmfs",
		"var/mmfs",

		// Mail spool
		"var/spool/postfix",

		// User dirs that can contain TB of crap
		"home",
		"/home/",
		"root/",

		// Backup detritus
		"*bak",
		"*old",
		"*tmp",
	}

	// Build exclude args
	var excludeArgs []string
	for _, dir := range excludeDirs {
		excludeArgs = append(excludeArgs, fmt.Sprintf("--exclude='%s'", dir))
	}
	
	tarCmd := fmt.Sprintf("sudo tar -czvf - %s /", strings.Join(excludeArgs, " "))
	
	// Check if sudo needed
	fmt.Print("sudo password: ")
	sudoPass, err := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return err
	}
	
	tarCmd = fmt.Sprintf("echo %s | sudo -S tar -czvf - %s /", string(sudoPass), strings.Join(excludeArgs, " "))
	
	b.logf("Starting clone of %s to %s", h.IP, dumpName)
	
	// Create output file
	outFile, err := os.Create(dumpName)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()
	
	// Execute SSH command and pipe to file
	cmd := exec.Command("sshpass", "-p", h.Password, "ssh",
		"-p", fmt.Sprintf("%d", h.RemotePort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-y",
		fmt.Sprintf("%s@%s", h.Username, h.IP),
		tarCmd)
	
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}
	
	b.logf("Clone completed: %s", dumpName)
	return nil
}

func (b *NEX) dynpf(hostSpec, remotePort string) error {
	h, err := b.formatHost(hostSpec)
	if err != nil {
		return err
	}
	
	b.logf("Starting dynamic port forward: %s:%s -> localhost:%s", h.IP, remotePort, remotePort)
	
	cmd := exec.Command("sshpass", "-p", h.Password, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-y",
		"-R", fmt.Sprintf("%s:localhost:%s", remotePort, remotePort),
		fmt.Sprintf("%s@%s", h.Username, h.IP),
		"sleep 3600")
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}
	
	b.logf("Port forward started (PID: %d)", cmd.Process.Pid)
	return nil
}

func expandBraces(pattern string) []string {
	// Let bash handle the expansion natively
	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo %s", pattern))
	output, err := cmd.Output()
	if err != nil {
		return []string{pattern}
	}
	
	// Split the output by spaces
	expanded := strings.Fields(strings.TrimSpace(string(output)))
	if len(expanded) == 0 {
		return []string{pattern}
	}
	
	return expanded
}

func (b *NEX) zapHosts(hostList string, command string) error {
	if command == "" {
		return fmt.Errorf("Usage: nex zap [host1,host2,...] <command>")
	}
	
	// Use bash to expand the host list natively
	hosts := expandBraces(hostList)
	
	if len(hosts) == 0 {
		return fmt.Errorf("no hosts specified")
	}
	
	// Check if sudo needed
	var sudoPass string
	if strings.Contains(command, "sudo") {
		fmt.Print("sudo password: ")
		pass, err := terminal.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return err
		}
		sudoPass = string(pass)
		command = fmt.Sprintf("echo %s | sudo -S %s", sudoPass, strings.TrimPrefix(command, "sudo "))
	}
	
	type result struct {
		host   string
		output string
		err    error
	}
	
	results := make(chan result, len(hosts))
	
	// Execute in parallel
	for _, hostSpec := range hosts {
		go func(hs string) {
			h, err := b.formatHost(hs)
			if err != nil {
				results <- result{host: hs, err: err}
				return
			}
			
			// Build command: hostname is ALWAYS prepended automatically
			// Redirect stderr to /dev/null to suppress warnings
			fullCmd := fmt.Sprintf("hostname ; %s 2>/dev/null", command)
			
			cmd := exec.Command("sshpass", "-p", h.Password, "ssh",
				"-p", fmt.Sprintf("%d", h.RemotePort),
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"-y",
				fmt.Sprintf("%s@%s", h.Username, h.IP),
				fullCmd)
			
			output, err := cmd.Output() // Use Output() instead of CombinedOutput() to ignore stderr
			if err != nil {
				results <- result{host: h.IP, output: "", err: err}
				return
			}
			
			// Convert all newlines to spaces for single-line output
			singleLine := strings.Join(strings.Fields(string(output)), " ")
			
			results <- result{host: h.IP, output: singleLine, err: nil}
		}(hostSpec)
	}
	
	// Collect and sort results
	var sortedResults []result
	for i := 0; i < len(hosts); i++ {
		r := <-results
		sortedResults = append(sortedResults, r)
	}
	
	// Sort by output (hostname is first word)
	for i := 0; i < len(sortedResults); i++ {
		for j := i + 1; j < len(sortedResults); j++ {
			if sortedResults[i].output > sortedResults[j].output {
				sortedResults[i], sortedResults[j] = sortedResults[j], sortedResults[i]
			}
		}
	}
	
	// Print results - one line per host
	for _, r := range sortedResults {
		if r.err != nil {
			fmt.Printf("%s: ERROR - %v\n", r.host, r.err)
		} else if r.output != "" {
			fmt.Println(r.output)
		}
	}
	
	return nil
}

func printHelp() {
	fmt.Println(`Usage: nex <command>

Commands:
  a/add                Add a host to DB
  build-db             Build database from YAML
  clone                Clone a host
  c/con/connect    	   Connect to a host
  d/del/delete         Delete a host from DB
  dump/dump-db         Dump database
  pf/dynpf             Dynamic port forward
  get                  Pull file from host
  put                  Push file to host
  ping                 Ping host
  socks	               Start SOCKS proxy
  ip                   Show IP
  z/zap                Execute command on multiple hosts`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}
	
	nex, err := NewNEX()
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer nex.Close()
	
	command := os.Args[1]
	
	switch command {
	case "a", "add":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex add -site=<site> -hostname=<hostname> -ipaddr=<ip> [-username=<user>] [-password=<pass>]")
		}
		// Parse flags
		h := Host{}
		for _, arg := range os.Args[2:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimPrefix(parts[0], "-")
			val := strings.Trim(parts[1], "\"")
			
			switch key {
			case "site":
				h.Site = val
			case "hostname":
				h.Hostname = val
			case "ipaddr":
				h.IP = val
			case "username":
				h.Username = val
			case "password":
				h.Password = val
			case "remote_port":
				fmt.Sscanf(val, "%d", &h.RemotePort)
			case "socks_port":
				fmt.Sscanf(val, "%d", &h.SocksPort)
			}
		}
		if err := nex.addHost(h); err != nil {
			log.Fatal(err)
		}
		
	case "build-db":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex build-db </path/to/yml>")
		}
		if err := nex.buildDB(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		
	case "clone":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex clone <host>")
		}
		if err := nex.cloneHost(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		
	case "c", "con", "connect":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex connect <host> [command]")
		}
		cmd := ""
		if len(os.Args) > 3 {
			cmd = strings.Join(os.Args[3:], " ")
		}
		if err := nex.connectHost(os.Args[2], cmd); err != nil {
			log.Fatal(err)
		}
		
	case "d", "del", "delete":
		if len(os.Args) < 4 {
			log.Fatal("Usage: nex delete -site=<site> -hostname=<hostname>")
		}
		var site, hostname string
		for _, arg := range os.Args[2:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimPrefix(parts[0], "-")
				val := strings.Trim(parts[1], "\"")
				if key == "site" {
					site = val
				} else if key == "hostname" {
					hostname = val
				}
			}
		}
		if err := nex.deleteHost(site, hostname); err != nil {
			log.Fatal(err)
		}
		
	case "dump", "dump-db":
		if err := nex.dumpDB(); err != nil {
			log.Fatal(err)
		}
		
	case "pf", "dynpf":
		if len(os.Args) < 4 {
			log.Fatal("Usage: nex dynpf <host> <remote_port>")
		}
		if err := nex.dynpf(os.Args[2], os.Args[3]); err != nil {
			log.Fatal(err)
		}
		
	case "get":
		if len(os.Args) < 4 {
			log.Fatal("Usage: nex get <host> <remote_file>")
		}
		if err := nex.getFile(os.Args[2], os.Args[3]); err != nil {
			log.Fatal(err)
		}

	case "install":
		// 'install' command now implicitly relies on NewNEX's logic
		// We can just exit if NewNEX succeeded, but running b.install() ensures schema setup on a new file.
		// NOTE: This will now fail if not run with sudo.
		if err := nex.install(); err != nil {
			log.Fatal(err)
		}

	case "put":
		if len(os.Args) < 4 {
			log.Fatal("Usage: nex put <host> <local_file>")
		}
		if err := nex.putFile(os.Args[2], os.Args[3]); err != nil {
			log.Fatal(err)
		}
		
	case "ping":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex ping <host>")
		}
		if err := nex.pingHost(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		
	case "shareids":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex shareids <host>")
		}
		if err := nex.shareIDs(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		
	case "s", "socks":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex socks <host>")
		}
		if err := nex.socksProxy(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		
	case "user":
		h, err := nex.formatHost(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(h.Username)
		
	case "pass":
		h, err := nex.formatHost(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(h.Password)
		
	case "ip":
		if len(os.Args) < 3 {
			log.Fatal("Usage: nex ip <host>")
		}
		h, err := nex.formatHost(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(h.IP)
		
	case "z", "zap":
		if len(os.Args) < 4 {
			log.Fatal("Usage: nex zap [ host1 host2 ... ] <command>\n   or: nex zap [host1,host2] <command>")
		}
		
		// Collect ALL args after 'zap'
		allArgs := os.Args[2:]
		
		var hostParts []string
		cmdStart := -1
		
		// Look for ] or find where command starts
		for i, arg := range allArgs {
			// Clean up brackets
			cleaned := strings.Trim(arg, "[]")
			
			// If we find ], everything after is command
			if arg == "]" || strings.HasSuffix(arg, "]") {
				if cleaned != "" && cleaned != "]" {
					hostParts = append(hostParts, cleaned)
				}
				cmdStart = i + 1
				break
			}
			
			// Skip standalone [
			if arg == "[" {
				continue
			}
			
			// If arg starts with [, remove it
			if strings.HasPrefix(arg, "[") {
				cleaned = strings.TrimPrefix(cleaned, "[")
			}
			
			// If this looks like a command (contains ;), stop collecting hosts
			if strings.Contains(arg, ";") {
				cmdStart = i
				break
			}
			
			if cleaned != "" {
				hostParts = append(hostParts, cleaned)
			}
		}
		
		if len(hostParts) == 0 || cmdStart == -1 || cmdStart >= len(allArgs) {
			log.Fatal("Usage: nex zap [ host1 host2 ... ] <command>")
		}
		
		// Join host parts with spaces for bash expansion
		hostList := strings.Join(hostParts, " ")
		command := strings.Join(allArgs[cmdStart:], " ")
		
		//fmt.Fprintf(os.Stderr, "DEBUG: hostList='%s' command='%s'\n", hostList, command)
		
		if err := nex.zapHosts(hostList, command); err != nil {
			log.Fatal(err)
		}
		
	default:
		printHelp()
	}
}
