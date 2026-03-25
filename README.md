# HashTasker

HashTasker is a high-performance distributed hash cracking system built in Go. It provides a web-based management interface for coordinating hashcat jobs across multiple worker nodes, enabling efficient password recovery and hash analysis at scale.

## Advantages Over Similar Tools

HashTasker offers several key advantages over existing solutions like Hashtopolis:

- **Multiple Concurrent Jobs**: Execute multiple hash cracking jobs simultaneously across your worker fleet, maximizing resource utilization
- **LDAP Authentication**: Enterprise-ready authentication with LDAP integration plus local admin fallback for flexible deployment scenarios
- **Modern Web Interface**: Clean, responsive dashboard built with modern web technologies for intuitive job management
- **Lightweight Architecture**: Single binary deployment with minimal dependencies and resource overhead
- **Real-time Monitoring**: Live progress tracking, performance metrics, and worker status monitoring
- **Flexible Worker Management**: Automatic worker discovery and load balancing with detailed worker statistics

## Features

### Core Functionality
- **Distributed Hash Cracking**: Coordinate hashcat operations across multiple worker nodes
- **Job Queue Management**: Automatic job scheduling and chunk distribution
- **Progress Tracking**: Real-time progress monitoring with ETA calculations
- **Result Management**: Centralized storage and retrieval of cracked hashes
- **Worker Load Balancing**: Intelligent work distribution based on worker capabilities

### Web Interface
- **Dashboard**: System overview with active jobs, worker status, and performance metrics
- **Job Management**: Create, monitor, and manage hash cracking jobs
- **Hash Management**: Upload hash files and view cracking results
- **User Management**: Admin panel for user account management
- **System Statistics**: Detailed performance metrics and historical data

### Authentication & Security
- **LDAP Integration**: Connect to existing Active Directory or LDAP servers
- **Local Admin Account**: Fallback authentication for standalone deployments
- **Session Management**: Secure session handling with configurable session keys
- **Role-based Access**: Administrative controls for user management

### API
- **Worker API**: RESTful endpoints for worker communication (no authentication required)
- **Management API**: Authenticated endpoints for web interface operations
- **Real-time Updates**: WebSocket-based live updates for job progress

## Building from Source

### Prerequisites
- Go 1.19 or later
- Git

### Build Instructions
```bash
git clone https://github.com/waffl3ss/HashTasker.git
cd HashTasker
go mod init hashtasker-go
go mod tidy
```

Build the server:
```bash
cd Server
go build -o hashtasker-server
```

Build the worker:
```bash
cd Worker
go build -o hashtasker-worker worker.go
```

## Required Files

### Server Requirements
The server needs the following files:
- `hashtasker-server` (binary)
- `config.yaml` (configuration file)
- `hashmodes.txt` (hashcat hash mode reference)
- `web/` directory (templates and static files)

### Worker Requirements  
Each worker needs:
- `hashtasker-worker` (binary)
- `worker.json` (configuration file)

## Configuration

### Server Configuration

Copy the example configuration file and modify it for your environment:
```bash
cp config.yaml.example config.yaml
```

#### Required Configuration Changes

**1. Session Key**
Generate a secure random session key:
```bash
openssl rand -hex 32
```
Update the `session_key` field in `config.yaml` with this value.

**2. Admin Password**
Change the default admin password in `admin.password`. This will be automatically hashed on first startup.

**3. Directory Paths**
Configure the following directories that must be accessible to the server:

- **Wordlist Directory** (`wordlists_path`): Directory containing wordlist files (.txt format)
- **Ruleset Directory** (`rulesets_path`): Directory containing hashcat rule files (.rule format)
- **Upload Directory** (`upload_path`): Directory for temporary file uploads
- **Static Files** (`static_path`): Path to web static files
- **Templates** (`template_path`): Path to web templates

**4. LDAP Configuration (Optional)**
If using LDAP authentication, configure:
```yaml
ldap:
  enabled: true
  host: "your-ldap-server.domain.com"
  port: 389
  bind_dn: "cn=admin,dc=company,dc=com"
  bind_pass: "your-ldap-password"
  base_dn: "ou=users,dc=company,dc=com"
  user_filter: "(uid=%s)"
  admin_group: "cn=admins,ou=groups,dc=company,dc=com"
```

### Example Server Configuration
```yaml
server:
  host: "0.0.0.0"
  port: 8080
  session_key: "your-secure-random-key"
  upload_path: "./uploads"
  static_path: "./web/static"
  template_path: "./web/templates"
  wordlists_path: "./wordlists"
  rulesets_path: "./rules"
  hashmodes_path: "./hashmodes.txt"

admin:
  username: "admin"
  password: "your-admin-password"

ldap:
  enabled: false
```

### Worker Configuration

Each worker node requires a `worker.json` configuration file:

```json
{
    "server_url": "http://your-server:8080",
    "hostname": "worker-01",
    "checkin_interval": "30s",
    "hashcat_path": "/usr/bin/hashcat",
    "rulesets_path": "/opt/rules",
    "temp_path": "/tmp/hashtasker"
}
```

#### Worker Configuration Fields

- **server_url**: Full URL to your HashTasker server
- **hostname**: Unique identifier for this worker node
- **checkin_interval**: How often to check for new jobs (e.g., "30s", "1m")
- **hashcat_path**: Path to the hashcat executable
- **rulesets_path**: Path to ruleset files (must match server rulesets)
- **temp_path**: Directory for temporary files and results

#### Worker Requirements

**On Worker Nodes:**
- Hashcat installed and accessible
- Same ruleset files as configured on the server (in the same directory structure)
- Network access to the HashTasker server
- Sufficient disk space for temporary files and results

**Ruleset Synchronization:**
Worker nodes must have the same ruleset files available as the server. Ensure the ruleset directory structure matches between server and workers.

## Running

### Start the Server
```bash
./hashtasker-server
```

The web interface will be available at `http://localhost:8080` (or your configured host/port).

### Start Workers
On each worker node:
```bash
./hashtasker-worker
```

Workers will automatically register with the server and begin polling for jobs.

## Default Credentials

- **Username**: admin
- **Password**: (as configured in config.yaml)

Change the default admin password immediately after installation.

## Security Considerations

- Use strong, unique passwords for all accounts
- Generate a cryptographically secure session key
- Use TLS/SSL in production environments
- Regularly update passwords and session keys
- Restrict network access to trusted hosts only
- Consider using LDAP authentication for centralized user management
- Remember to exclude your real `config.yaml` from version control

## License

This project is licensed under the MIT License - see the LICENSE file for details.
