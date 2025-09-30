# HashTasker Changelog

All notable changes to HashTasker will be documented in this file.

## v1.0.6 - 2025-09-29

### ✅ Added
- **Hash Whitespace Stripping**: Automatic whitespace removal from hash lines across all input methods
- **Comprehensive Hash Processing**: All hash operations now strip leading/trailing whitespace from each line

### 🔄 Changed
- **Hash Text Input**: Pasted hash text now has whitespace stripped from each line before storage
- **Hash File Upload**: Uploaded hash files are processed to remove whitespace from each line during upload
- **Job Copying**: Hash text in copied jobs now has whitespace stripped to ensure consistency
- **Worker Hash Files**: Worker-created hash files receive clean, whitespace-stripped content
- **Database Storage**: Hash text stored in database has whitespace properly stripped from each line

### 🛠️ Technical Improvements
- **New `processHashContent()` Function**: Centralizes hash content processing logic
- **Enhanced File Processing**: Upload handler now processes file content instead of direct file saving
- **Consistent Line Processing**: All hash input methods now use unified `splitLines()` processing

## v1.0.5 - 2025-09-28

### ✅ Added
- **Comprehensive File Cleanup System**: Automatic cleanup of temporary files when jobs finish
- **Worker File Management**: Workers now clean up hash files, wordlist chunks, and pot files when jobs complete/fail/cancel
- **Server Chunk Cleanup**: Server automatically removes wordlist chunks when jobs reach final states
- **Orphaned Job Protection**: Workers skip and track invalid job assignments with empty UIDs to prevent endless loops
- **Memory Management**: Worker cleanup routine prevents infinite memory growth from processed jobs map

### 🔄 Changed
- **Worker Error Handling**: Enhanced validation to skip assignments with empty job UIDs
- **Job Status Updates**: Cleanup triggers automatically when jobs complete, fail, or are cancelled
- **Handler Architecture**: WorkerHandler now accepts upload path for proper cleanup functionality

### 🐛 Fixed
- **Worker Log Spam**: Fixed endless "Skipping job _0 - already processed" messages from corrupted job data
- **Memory Leaks**: Added periodic cleanup of worker processed jobs map to prevent unbounded growth
- **Temporary File Accumulation**: All temporary chunk files now properly cleaned up when no longer needed

### 🎨 Performance Improvements
- **Disk Space Management**: Automatic cleanup prevents unnecessary disk usage from old job chunks
- **Worker Efficiency**: Workers no longer waste cycles processing invalid job assignments
- **Asynchronous Cleanup**: File cleanup runs in background to avoid blocking job operations

---

## v1.0.4 - 2025-09-26

### ✅ Added
- **Line Numbers in Hash Input**: Added line numbers to the hash paste box on job creation page for easier reference
- **Troubleshooting Page**: New troubleshooting guide accessible from navigation menu
- **Changelog Page**: Version history accessible from footer link
- **Dynamic Content Loading**: Troubleshooting and changelog pages now read from markdown files without requiring server restart
- **Enhanced Job Status Tracking**: Added "overloaded" status for better worker capacity management
- **Cross-Platform Text Handling**: Improved newline normalization for Windows/Unix compatibility

### 🔄 Changed
- **Database Connection**: Enhanced MySQL driver integration with GORM
- **Worker Management**: Improved worker registration and status tracking with 60-second timeout
- **Job Processing**: Enhanced wordlist chunk distribution system across worker nodes
- **Status Management**: More granular job and chunk status tracking with completion detection

### 🐛 Fixed
- **Text Format Issues**: Fixed cross-platform compatibility for different line ending formats (CRLF → LF)
- **Worker Offline Detection**: Automatic worker offline status after 60 seconds of inactivity
- **Job Completion Logic**: Improved job status aggregation based on chunk completion
- **Connection Pooling**: Optimized database connections (max idle: 10, max open: 100)

### 🎨 UI/UX Improvements
- **Navigation**: Added troubleshooting link to main navigation
- **Footer**: Version number now links to changelog
- **Hash Editor**: Monospace font with line numbers for better hash management
- **Responsive Design**: Enhanced mobile compatibility

---

## v1.0.3 - Previous Release

### ✅ Added
- **Multiple Concurrent Jobs**: Execute multiple hash cracking jobs simultaneously
- **LDAP Authentication**: Enterprise-ready authentication with local admin fallback
- **Real-time Monitoring**: Live progress tracking and worker status monitoring
- **Worker Load Balancing**: Intelligent work distribution based on capabilities

### 🔄 Changed
- **Web Interface**: Modern responsive dashboard with Bootstrap 5
- **API Structure**: RESTful endpoints for all operations
- **Job Management**: Enhanced job queue and chunk distribution

### 🐛 Fixed
- **Session Management**: Improved session handling and security
- **Performance**: Optimized database queries and file operations

---

## v1.0.2 - Earlier Release

### ✅ Added
- **Hash Management**: Centralized storage and retrieval of cracked hashes
- **User Management**: Admin panel for user account management
- **System Statistics**: Detailed performance metrics and historical data

### 🔄 Changed
- **Architecture**: Single binary deployment with minimal dependencies
- **Security**: Enhanced authentication and session management

---

## v1.0.1 - Initial Release

### ✅ Added
- **Core Functionality**: Distributed hash cracking coordination
- **Worker Communication**: RESTful API for worker nodes
- **Basic Web Interface**: Job creation and monitoring
- **SQLite Support**: Local database for development and small deployments

---

## Future Roadmap

### Planned Features
- **Advanced Scheduling**: Cron-like job scheduling
- **Result Export**: Multiple format support for cracked hashes
- **Advanced Analytics**: Detailed performance and success metrics
- **Plugin System**: Custom attack modules and integrations
- **Clustering**: Advanced worker grouping and resource allocation

### Under Consideration
- **REST API v2**: Enhanced API with pagination and filtering
- **Webhooks**: Job completion notifications
- **Advanced Security**: Two-factor authentication, audit logging
- **Cloud Integration**: Support for cloud-based workers

---

## Notes

- This changelog follows [Keep a Changelog](https://keepachangelog.com/) format
- Version numbers follow [Semantic Versioning](https://semver.org/)
- Dates are in YYYY-MM-DD format
- This file is automatically loaded by HashTasker - no server restart required for updates