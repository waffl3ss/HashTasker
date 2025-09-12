# HashTasker

HashTasker is a high-performance distributed hash cracking system built in Go. It provides a web-based management interface for coordinating hashcat jobs across multiple worker nodes, enabling efficient password recovery and hash analysis at scale. I made this as I wanted to have ldap authentication for my team, as well as multiple jobs at once instead of one job at a time. I'm well aware that this will slow down jobs, but in timed engagements, theres no room to wait for other jobs to finish.

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
- **Worker API**: RESTful endpoints for worker communication 
- **Management API**: Authenticated endpoints for web interface operations
- **Real-time Updates**: WebSocket-based live updates for job progress

## Notes
- You can run the worker on however many systems you want, when it calls back to the server, it will register as ready to use for jobs.
- The binary is embed with the web templates and files, so no need to include the web directory on the server
- its fairly straight forward. feel free to reach out if you have issues.

## Building from Source

### Build Instructions (go 1.19+ required)
```bash
git clone https://github.com/waffl3ss/HashTasker.git
cd HashTasker
go mod init hashtasker-go
go mod tidy
go build -o hashtasker-server server.go
go build -o hashtasker-worker worker.go
```

### Releases
Pre-compiled binaries are available in the [Releases](https://github.com/waffl3ss/HashTasker/releases) section.

## Required Files

### Server Requirements
The server needs the following files:
- `hashtasker-server` (binary)
- `config.yaml` (configuration file)
- `hashmodes.txt` (hashcat hash mode reference)

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
Change the default admin password in the `config.yaml`. This will be automatically hashed on first startup.

**3. Directory Paths**
Configure the following directories that must be accessible to the server:

- **Wordlist Directory** (`wordlists_path`): Directory containing wordlist files
- **Ruleset Directory** (`rulesets_path`): Directory containing hashcat rule files 
- **Upload Directory** (`upload_path`): Directory for temporary file uploads
- **Static Files** (`static_path`): Path to web static files (leave default unless you change things, binary build has these embeded)
- **Templates** (`template_path`): Path to web templates (leave default unless you change things, binary build has these embeded)

**4. LDAP Configuration (Optional)**
If using LDAP authentication, configure the `config.yaml` with the proper settings. there is an `admin_group` that you can set for administrators of the platform. 

### Worker Configuration

Each worker node requires a `worker.json` configuration file:

```json
{
    "server_url": "http://your-server:8080",
    "hostname": "worker-01",
    "checkin_interval": "30s",
    "hashcat_path": "/usr/bin/hashcat/hashcat.bin",
    "rulesets_path": "/opt/rules",
    "temp_path": "/tmp/hashtasker"
}
```

#### Worker Configuration Fields

- **server_url**: Full URL to your HashTasker server
- **hostname**: Unique identifier for this worker node
- **checkin_interval**: How often to check for new jobs (e.g., "30s", "1m")
- **hashcat_path**: Path to the hashcat executable
- **rulesets_path**: Path to ruleset files (must be the same list of rules that the server has)
- **temp_path**: Directory for temporary files and results

#### Worker Requirements

**On Worker Nodes:**
- Hashcat.bin/rulesets/etc accessible
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

The web interface will be available at `http://server-ip:8080` (or your configured host/port).

### Start Workers
On each worker node:
```bash
./hashtasker-worker -config worker.json
```

Workers will automatically register with the server and begin polling for jobs.

## Default Credentials

- **Username**: admin
- **Password**: (as configured in config.yaml)

- If you have ldap setup, you can start logging in with that, any user within the admin group detailed by the ldap settings will have admin priviledges.
