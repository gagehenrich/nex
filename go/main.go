package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const dbFile = ".nex/nex.db3"

func printDb(db *sql.DB) {
	fmt.Println("Hosts Table:")
	rows, err := db.Query("SELECT id, site, hostname, ipaddr, username, password, sudo_password, remote_port, socks_port FROM hosts")
	if err != nil {
		log.Fatalf("Failed to query hosts: %v", err)
	}
	defer rows.Close()

	fmt.Printf("%-5s %-10s %-20s %-15s %-15s %-15s %-15s %-12s %-12s\n", "ID", "Site", "Hostname", "IP Address", "Username", "Password", "Sudo Password", "Remote Port", "Socks Port")
	for rows.Next() {
		var id int
		var site, hostname, ipaddr, username, password, sudoPassword string
		var remotePort, socksPort int
		if err := rows.Scan(&id, &site, &hostname, &ipaddr, &username, &password, &sudoPassword, &remotePort, &socksPort); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		fmt.Printf("%-5d %-10s %-20s %-15s %-15s %-15s %-15s %-12d %-12d\n", id, site, hostname, ipaddr, username, password, sudoPassword, remotePort, socksPort)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}
}

func install() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}
	dbFileExpanded := homeDir + "/" + dbFile

	db, err := sql.Open("sqlite3", dbFileExpanded)
	if err != nil {
		log.Fatalf("Failed to open database file: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
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
    sudo_password TEXT,
    remote_port INTEGER,
    socks_port INTEGER
);`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("SQLite3 database created at %s\n", dbFileExpanded)
}

func addHost(db *sql.DB, site, hostname, ipaddr, username, password, sudoPassword string, remotePort, socksPort int) {
	_, err := db.Exec(`INSERT INTO hosts (site, hostname, ipaddr, username, password, sudo_password, remote_port, socks_port) 
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		site, hostname, ipaddr, username, password, sudoPassword, remotePort, socksPort)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Host added successfully.")
}

func updateHost(db *sql.DB, site, hostname, ipaddr, username, password, sudoPassword string, remotePort, socksPort int) {
	_, err := db.Exec(`UPDATE hosts SET ipaddr=?, username=?, password=?, sudo_password=?, remote_port=?, socks_port=? 
	WHERE site=? AND hostname=?`,
		ipaddr, username, password, sudoPassword, remotePort, socksPort, site, hostname)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Host updated successfully.")
}

func listHosts(db *sql.DB) {
	rows, err := db.Query("SELECT site, hostname FROM hosts")
	if err != nil {
		log.Fatalf("Failed to query hosts: %v", err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var site, hostname string
		if err := rows.Scan(&site, &hostname); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		results = append(results, fmt.Sprintf("%s:%s", site, hostname))
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	fmt.Println(strings.Join(results, " "))
}

func queryHost(db *sql.DB, site, hostname string) (map[string]string, error) {
	row := db.QueryRow(`
	SELECT ipaddr, username, password, sudo_password, remote_port, socks_port
	FROM hosts WHERE site = ? AND hostname = ?`, site, hostname)

	var ipaddr, username, password, sudoPassword string
	var remotePort, socksPort int
	err := row.Scan(&ipaddr, &username, &password, &sudoPassword, &remotePort, &socksPort)
	if err != nil {
		return nil, err
	}

	hostDetails := map[string]string{
		"ipaddr":         ipaddr,
		"username":       username,
		"password":       password,
		"sudo_password":  sudoPassword,
		"remote_port":    fmt.Sprint(remotePort),
		"socks_port":     fmt.Sprint(socksPort),
	}

	return hostDetails, nil
}

func usage() {
	fmt.Println("Usage: manage_db {install|addHost|updateHost|addConfig|queryHost|updateConfig} [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  install - Create the SQLite3 database and tables")
	fmt.Println("  addHost - Add a new host")
	fmt.Println("  updateHost - Update an existing host")
	fmt.Println("  addConfig - Add a new host entry")
	fmt.Println("  queryHost - query host entry")
	fmt.Println("  updateConfig - Update an existing host entry")
	fmt.Println("")
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	command := os.Args[1]
	args := os.Args[2:]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}
	dbFileExpanded := homeDir + "/" + dbFile

	err = os.MkdirAll(homeDir+"/.nex", os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	db, err := sql.Open("sqlite3", dbFileExpanded)
	if err != nil {
		log.Fatalf("Failed to open database file: %v", err)
	}
	defer db.Close()

	switch command {
	case "install":
		install()
	case "addHost":
		flags := flag.NewFlagSet("addHost", flag.ExitOnError)
		site := flags.String("site", "", "Site")
		hostname := flags.String("hostname", "", "Hostname")
		ipaddr := flags.String("ipaddr", "", "IP Address")
		username := flags.String("username", "", "Username")
		password := flags.String("password", "", "Password")
		sudoPassword := flags.String("sudo_password", "", "Sudo Password")
		remotePort := flags.Int("remote_port", 0, "Remote Port")
		socksPort := flags.Int("socks_port", 0, "Socks Port")
		flags.Parse(args)
		addHost(db, *site, *hostname, *ipaddr, *username, *password, *sudoPassword, *remotePort, *socksPort)
	case "updateHost":
		flags := flag.NewFlagSet("updateHost", flag.ExitOnError)
		site := flags.String("site", "", "Site")
		hostname := flags.String("hostname", "", "Hostname")
		ipaddr := flags.String("ipaddr", "", "IP Address")
		username := flags.String("username", "", "Username")
		password := flags.String("password", "", "Password")
		sudoPassword := flags.String("sudo_password", "", "Sudo Password")
		remotePort := flags.Int("remote_port", 0, "Remote Port")
		socksPort := flags.Int("socks_port", 0, "Socks Port")
		flags.Parse(args)
		updateHost(db, *site, *hostname, *ipaddr, *username, *password, *sudoPassword, *remotePort, *socksPort)
	case "listHosts":
		listHosts(db)
	case "printDb":
		printDb(db)
	case "queryHost":
		flags := flag.NewFlagSet("queryHost", flag.ExitOnError)
		site := flags.String("site", "", "Site")
		hostname := flags.String("hostname", "", "Hostname")
		flags.Parse(args)
		hostDetails, err := queryHost(db, *site, *hostname)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(hostDetails)
	default:
		usage()
	}
}
