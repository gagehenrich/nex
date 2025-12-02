# 🚀 **NEX**
### *The Ultimate SSH Operations Toolkit — Fast. Secure. Zero Bullshit.*

**NEX** is a high-velocity SSH workflow engine built for engineers who manage fleets, not single servers.  
Think of it as a **Swiss-Army knife for distributed Linux ops** — with encrypted credentials, parallel execution, file transfers, SOCKS tunnels, cloning, and YAML-driven platform imports.

It does everything you wish SSH did natively.

---

# ⚡ Core Capabilities

## 🛠️ Swiss Army Knife for SSH Ops
- Instant SSH connections  
- Remote command execution  
- Passwordless SCP transfers (get/put)  
- SOCKS proxy tunneling  
- Dynamic port forwarding  
- SSH key sharing  
- Full system cloning  
- Network diagnostics (ping, IP lookup)  
- Site-aware autocomplete & addressing  

---

# 🔥 Quick Examples

### **Instantly connect**
```bash
$ nex connect customer-02:cache-01
[username@cache-01 ~]$
```

### **Run commands remotely**
```bash
$ nex c customer-02\:srv-01 'sudo dmesg | grep -i fail'
```

### **Blast commands across entire platforms**
```bash
$ nex zap [ customer-02:srv-{01..16} ] 'uptime'
```

### **Transfer files effortlessly**
```bash
nex get 10.20.100.101 /var/log/messages
nex put customer-01:srv-01 ./installer.tar.gz
```

---

# 🛡️ Security Model

### 🔐 Zero-Compromise Design
- **SQLCipher AES-256 encrypted database**  
- **Credentials encrypted at build time** (no plaintext ever written)  
- **Setuid root binary** grants controlled, safe passwordless operations  
- **No credential leakage** — not in RAM, not on disk, nowhere  

---

# ⚡ Performance Features

- **Parallel execution engine** (`nex zap`)  
  Execute commands across *hundreds* of servers with ordered output.  
- **Smart caching**  
  Avoid redundant DB lookups.  
- **Instant hostname resolution**  
  Use `site:hostname`, wildcards, ranges, or raw IPs.

---

# 📦 Installation

```bash
git clone https://github.com/gagehenrich/nex.git
cd nex
make
```

### Installer does the magic:
1. Builds binary with encrypted credentials  
2. Creates encrypted DB: `/var/lib/nex/nex.db3`  
3. Installs bash autocomplete  
4. Applies `setuid` for passwordless execution  

---

# 🚀 Quick Start

### 1. Edit platform YAML

`./template/platform.yml`:
```yaml
all:
  username: default_user
  password: default_pass
  socks_port: 9999
  remote_port: 22

sites:
  customer1:
    hosts:
      srv-01:
        ipaddr: 192.168.1.10
      srv-02:
        ipaddr: 192.168.1.11
      srv-03:
        ipaddr: 192.168.1.20
        username: special_user

  customer2:
    hosts:
      host-01:
        ipaddr: 10.0.1.50
      host-02:
        ipaddr: 10.0.1.51
```

### 2. Import into NEX
```bash
nex build-db ./template/platform.yml
```

### 3. Verify installation
```bash
nex dump-db
nex ping customer1:srv-01
nex ip customer2:host-02
```

---

# 📚 Usage Guide

## Connect to a host
```bash
nex connect customer1:srv-01
nex connect customer1:srv-01 "systemctl status nginx"
nex connect 192.168.1.10
```

## File transfers
```bash
nex get customer1:srv-01 /var/log/nginx/access.log
nex put customer1:srv-01 ./config.yaml
```

## Network operations
```bash
nex ping customer1:srv-01
nex socks customer1:srv-01
nex dynpf customer1:srv-01 8080
```

## SSH key sharing
```bash
nex shareids customer1:srv-01
```

## Parallel execution (ZAP)
```bash
nex z [ 10.31.100.1{01..04} ] "cat /etc/redhat-release ; uname -r"
```

## Host management
```bash
nex add -site=customer1 -hostname=srv-01 -ipaddr=192.168.1.30
nex delete -site=customer1 -hostname=cache-01
nex ip customer1:srv-01
nex user customer1:srv-01
nex pass customer1:srv-01
```

## System cloning
```bash
nex clone customer1:srv-01
# Output: 192.168.1.10.tgz
```

---

# 🏗️ Architecture

## Security Flow
```
Build-Time
  Credentials → AES Encryption → Compiled Binary
      ↓
Runtime
  Binary (setuid root) → SQLCipher Key → Encrypted DB
      ↓
Filesystem
  /var/lib/nex/nex.db3
```

## File Layout
```
/usr/local/bin/nex            # setuid binary
/var/lib/nex/nex.db3          # encrypted database
/var/lib/nex/autocomplete.sh  # bash completion
~/.bashrc                      # sources completion
```

## Database Schema
```sql
CREATE TABLE hosts (
  id INTEGER PRIMARY KEY,
  site TEXT NOT NULL,
  hostname TEXT NOT NULL,
  ipaddr TEXT NOT NULL,
  username TEXT,
  password TEXT,
  remote_port INTEGER,
  socks_port INTEGER,
  UNIQUE(site, hostname)
);

CREATE TABLE configurations (
  id INTEGER PRIMARY KEY,
  key TEXT UNIQUE NOT NULL,
  value TEXT NOT NULL
);
```

---

# ⚙️ Configuration

### Makefile variables
```makefile
BIN_NAME    := nex
LOCAL_DIR   := "/var/lib/nex"
DEPS        := sshpass openssl sqlcipher-devel
```

### Enable autocomplete
```bash
source ~/.bashrc
nex conn<TAB>
```

---

# 🩺 Troubleshooting

### “Incorrect password or corrupted database”
Rebuild DB:
```bash
make empty-db
make load-db
```

### SSH issues
```bash
nex ping site:hostname
nex user site:hostname
nex pass site:hostname
nex connect site:hostname "echo test"
```

---

# 💪 Contributing
PRs welcome. Issues welcome.  
If you build something cool with NEX — **show it off**.

---

# 📄 License
GPL-3
