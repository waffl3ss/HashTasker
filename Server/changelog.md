# HashTasker Changelog

All notable changes to HashTasker will be documented in this file.

## v1.2.0 - 2026-03-24

### New Features
- **Auto-Delete Old Jobs**: Jobs older than 45 days are automatically deleted along with all associated files and database records
  - Runs on server startup and every 12 hours
  - Retroactive — cleans up existing old jobs immediately on upgrade
- **NTDS Job Detection**: Jobs with hash mode 1000 (NTLM) and over 300 hashes are automatically flagged as NTDS
  - Existing jobs retroactively flagged on startup
  - NTDS flag stored on the job record for consistent filtering
- **NTDS Analytics Filter**: Analytics dashboard excludes NTDS jobs by default to prevent skewed statistics
  - Toggle button to include/exclude NTDS data
  - All analytics (summary, charts, wordlist/ruleset/user/hash mode usage) respect the filter
  - Analytics now computed from raw job data instead of pre-aggregated tables for accurate filtering
- **Admin Jobs Toggle**: Administrators can toggle between viewing their own jobs and all users' jobs on the Jobs page
  - Toggle button visible only to admins, placed next to the search box
  - Defaults to showing only the current user's jobs
- **Username Mode**: Toggle for hashcat's `--username` flag when hashes are in `username:hash` format
  - Checkbox on job creation form next to Disable Potfile
  - Clear warning text to prevent misuse on hashes that naturally contain colons
  - Setting preserved when copying jobs
- **Webhook Notifications**: Optional webhook URL on job creation for real-time status updates
  - Sends POST requests with `{"msg": "..."}` payload for major job events
  - Notifies on job creation, running, completed, failed, and cancelled
  - Webhook URL is preserved when copying jobs
- **Orphaned File Cleanup**: Background task removes files in the uploads directory that belong to deleted/missing jobs
  - Runs on startup and every hour
  - Detects orphans by matching file naming patterns against the database

### Improved File Cleanup
- **Comprehensive Job File Cleanup**: When a job completes, fails, or is cancelled, all associated files are now cleaned up — wordlist chunks, custom uploaded wordlists, and potfiles
  - Previously only wordlist chunks were cleaned; custom wordlists and potfiles were left behind
- **Cancel Job Cleanup**: Cancelling a job now also removes custom uploaded wordlists and potfiles (previously only removed chunks)
- **Shared Purge Logic**: Manual delete and auto-delete use the same `purgeJob()` function for consistent cleanup

### UI Improvements
- **Analytics Card Heights**: Summary cards on the analytics page now have equal heights
- **Job Details Card Heights**: Job Information and Progress Overview cards now have equal heights

### Database Maintenance
- **Orphaned Record Cleanup**: On startup, removes orphaned `job_chunks`, `job_progresses`, and `cracked_hashes` records whose parent job was deleted
  - Eliminates "Found job chunk with empty job UID" log warnings
  - Runs automatically during migration on every boot

### Removed
- **Troubleshooting Page**: Removed the troubleshooting page, navigation link, route, and associated markdown file

---

## v1.1.6 - 2025-12-16

### New Features
- **Clear Analytics Data**: Admins can now clear all analytics data with triple confirmation
  - Removes all historical job statistics, usage data, and records
  - Requires 3 confirmation prompts to prevent accidental deletion

### UI Improvements
- **Recovered Hashes Table**: Card now adjusts to content height instead of fixed scrollable area
  - Table grows with data (safe since pagination limits to 50 hashes per page)
  - Better user experience without unnecessary scroll bars

## v1.1.5 - 2025-12-15

### New Features
- **Admin Analytics Dashboard**: Comprehensive analytics page for administrators
  - Total jobs, hashes submitted, and hashes cracked with crack rate percentage
  - Jobs over time chart with daily/weekly/monthly/yearly views
  - Top wordlists usage tracking
  - Top rulesets usage tracking
  - Hash mode usage breakdown

### UI Improvements
- **Uniform Modal Dialogs**: Replaced all browser alert()/confirm() dialogs with styled Bootstrap modals
  - Consistent styling across all pages (job creation, deletion, user management, etc.)
  - Dark mode compatible
- **Login Page Dark Mode**: Background color now matches the dark grey theme of other pages
- **Blank Password Display**: Empty/blank cracked passwords now show "[BLANK PASSWORD]" instead of empty cell
- **Cracked Hashes Pagination**: Fixed "Showing 0-0 of 0 hashes" to display accurate counts even with single page results

### Job Creation Improvements
- **Sorted Wordlists**: Wordlists now sorted by file size (largest first) in dropdown
- **Sorted Rulesets**: Rulesets now sorted by file size (largest first) in dropdown
- **Ruleset Size Display**: Ruleset dropdown now shows file size alongside rule count

### Technical Changes
- Added 6 new database tables for analytics (auto-migrated on startup)
- Analytics recorded automatically on job creation and hash cracking
- Admin-only access control for analytics page and API

## v1.1.4 - 2025-10-07

### Worker Overload Detection Improvements
- **Hashcat-Based Overload Detection**: Workers now rely on hashcat's exit code 252 for overload detection
  - Removed CPU-based pre-check limits that artificially restricted workers
  - Workers attempt to start jobs and let hashcat determine if system is overloaded
  - When hashcat returns exit code 252, job is queued locally and retried when capacity available
  - Retry logic waits for process count to drop by at least 1 before attempting again
  - Prevents duplicate job attempts while in overloaded retry queue

### Cracked Hash Deduplication
- **Database-Level Deduplication**: Implemented comprehensive solution to prevent duplicate cracked hashes
  - Added unique constraint on job_id + hash combination
  - API checks for existing hash before insertion
  - Migration automatically cleans up existing duplicate entries
  - Cracked counts now use DISTINCT queries for accuracy
  - Fixes issue where "4 of 3 hashes cracked" could appear
  - Pie charts and progress indicators now show correct percentages

### Display Improvements
- **Alphabetical Worker Ordering**: Workers now displayed in alphabetical order
  - Job details page worker progress section sorted alphabetically
  - System stats page workers sorted alphabetically
  - Consistent ordering: cracker1, cracker2, cracker3, etc.

### UI Polish
- **Dark Mode Alert Visibility**: Fixed "Worker Queued" alert visibility in dark mode
  - Improved background color contrast
  - More readable text colors
  - Better border visibility

## v1.1.3 - 2025-10-06

### Worker Status Display Improvements
- **Queued Worker Visibility**: Workers now properly report and display "queued" status when overloaded
  - Workers report status to server when waiting for capacity
  - UI displays informative message explaining worker is waiting
  - Exit code 252 (overloaded) now triggers proper status reporting

### Enhanced Progress Display
- **Active Worker Count**: Jobs page now shows "X of Y workers active" instead of "Preparing workers..."
  - Always displays active worker count (e.g., "1 of 2 workers active" or "0 of 2 workers active")
  - More informative for users monitoring job progress

### Time Remaining Calculation Fix
- **Accurate Time Estimates**: Time remaining now properly averages only running workers
  - If worker1 is queued and worker2 shows 5h remaining, total displays 5h
  - If worker1 shows 5h and worker2 shows 10h, total displays 7.5h (average)
  - Queued workers excluded from time calculations until they start running

### UI Polish
- **Worker Status Badges**: Job details page displays clear status for each worker
  - Queued workers show info badge with waiting message
  - Running workers show progress bars and time estimates
  - Completed workers show success status

## v1.1.1 - 2025-10-01

### Bug Fixes
- **Wordlist Buffer Size**: Fixed "bufio.Scanner: token too long" error by increasing buffer capacity to 10MB
  - Handles wordlists with extremely long lines (up to 10MB per line)
  - Applied to both line counting and wordlist splitting operations

### UI Improvements
- **Dark Mode File Upload**: Fixed file upload info boxes to properly match dark mode styling
  - Consistent colors across hash file and wordlist file uploads
  - Improved visibility of close buttons in dark mode

## v1.1 - 2025-10-01

### Added
- **CSV Export**: Export cracked hashes to CSV format with Hash, Password, and Recovery Time columns
  - Export cracked hashes only
  - Export all hashes (cracked and uncracked)
- **DPAT Potfile Export**: Generate hashcat-compatible potfiles for NTLM jobs (mode 1000)
  - Download button appears automatically for NTLM jobs
  - Standard hash:password format for DPAT compatibility
- **Improved Hash File Upload**: Hash files are now read and stored in database like pasted hashes
  - Eliminates worker file access issues
  - Consistent processing across all input methods
- **Export Dropdown Menu**: Organized export options with sections for CSV and DPAT formats

### Changed
- **Hash File Processing**: Uploaded hash files are now processed into HashesText field instead of storing as files
- **Worker Assignment**: Workers now receive hash content directly, not file paths
- **Export Button**: Renamed from "Export CSV" to "Export" with categorized dropdown options

### Technical Improvements
- **Unified Hash Storage**: All hashes stored in HashesText field regardless of input method
- **CSV Field Escaping**: Proper escaping for CSV fields containing commas, quotes, or newlines
- **Automatic Cleanup**: Generated potfiles are cleaned up when jobs are deleted

## v1.0.6 - 2025-09-29

### Added
- **Hash Whitespace Stripping**: Automatic whitespace removal from hash lines across all input methods
- **Comprehensive Hash Processing**: All hash operations now strip leading/trailing whitespace from each line

### Changed
- **Hash Text Input**: Pasted hash text now has whitespace stripped from each line before storage
- **Hash File Upload**: Uploaded hash files are processed to remove whitespace from each line during upload
- **Job Copying**: Hash text in copied jobs now has whitespace stripped to ensure consistency
- **Worker Hash Files**: Worker-created hash files receive clean, whitespace-stripped content
- **Database Storage**: Hash text stored in database has whitespace properly stripped from each line

### Technical Improvements
- **New `processHashContent()` Function**: Centralizes hash content processing logic
- **Enhanced File Processing**: Upload handler now processes file content instead of direct file saving
- **Consistent Line Processing**: All hash input methods now use unified `splitLines()` processing

## v1.0.5 - 2025-09-28

### Added
- **Comprehensive File Cleanup System**: Automatic cleanup of temporary files when jobs finish
- **Worker File Management**: Workers now clean up hash files, wordlist chunks, and pot files when jobs complete/fail/cancel
- **Server Chunk Cleanup**: Server automatically removes wordlist chunks when jobs reach final states
- **Orphaned Job Protection**: Workers skip and track invalid job assignments with empty UIDs to prevent endless loops
- **Memory Management**: Worker cleanup routine prevents infinite memory growth from processed jobs map

### Changed
- **Worker Error Handling**: Enhanced validation to skip assignments with empty job UIDs
- **Job Status Updates**: Cleanup triggers automatically when jobs complete, fail, or are cancelled
- **Handler Architecture**: WorkerHandler now accepts upload path for proper cleanup functionality

### Bug Fixes
- **Worker Log Spam**: Fixed endless "Skipping job _0 - already processed" messages from corrupted job data
- **Memory Leaks**: Added periodic cleanup of worker processed jobs map to prevent unbounded growth
- **Temporary File Accumulation**: All temporary chunk files now properly cleaned up when no longer needed

### Performance Improvements
- **Disk Space Management**: Automatic cleanup prevents unnecessary disk usage from old job chunks
- **Worker Efficiency**: Workers no longer waste cycles processing invalid job assignments
- **Asynchronous Cleanup**: File cleanup runs in background to avoid blocking job operations

---

## v1.0.4 - 2025-09-26

### Added
- **Line Numbers in Hash Input**: Added line numbers to the hash paste box on job creation page for easier reference
- **Troubleshooting Page**: New troubleshooting guide accessible from navigation menu
- **Changelog Page**: Version history accessible from footer link
- **Dynamic Content Loading**: Troubleshooting and changelog pages now read from markdown files without requiring server restart
- **Enhanced Job Status Tracking**: Added "overloaded" status for better worker capacity management
- **Cross-Platform Text Handling**: Improved newline normalization for Windows/Unix compatibility

### Changed
- **Database Connection**: Enhanced MySQL driver integration with GORM
- **Worker Management**: Improved worker registration and status tracking with 60-second timeout
- **Job Processing**: Enhanced wordlist chunk distribution system across worker nodes
- **Status Management**: More granular job and chunk status tracking with completion detection

### Bug Fixes
- **Text Format Issues**: Fixed cross-platform compatibility for different line ending formats (CRLF → LF)
- **Worker Offline Detection**: Automatic worker offline status after 60 seconds of inactivity
- **Job Completion Logic**: Improved job status aggregation based on chunk completion
- **Connection Pooling**: Optimized database connections (max idle: 10, max open: 100)

### UI/UX Improvements
- **Navigation**: Added troubleshooting link to main navigation
- **Footer**: Version number now links to changelog
- **Hash Editor**: Monospace font with line numbers for better hash management
- **Responsive Design**: Enhanced mobile compatibility

---

## v1.0.3 - Previous Release

### Added
- **Multiple Concurrent Jobs**: Execute multiple hash cracking jobs simultaneously
- **LDAP Authentication**: Enterprise-ready authentication with local admin fallback
- **Real-time Monitoring**: Live progress tracking and worker status monitoring
- **Worker Load Balancing**: Intelligent work distribution based on capabilities

### Changed
- **Web Interface**: Modern responsive dashboard with Bootstrap 5
- **API Structure**: RESTful endpoints for all operations
- **Job Management**: Enhanced job queue and chunk distribution

### Bug Fixes
- **Session Management**: Improved session handling and security
- **Performance**: Optimized database queries and file operations

---

## v1.0.2 - Earlier Release

### Added
- **Hash Management**: Centralized storage and retrieval of cracked hashes
- **User Management**: Admin panel for user account management
- **System Statistics**: Detailed performance metrics and historical data

### Changed
- **Architecture**: Single binary deployment with minimal dependencies
- **Security**: Enhanced authentication and session management

---

## v1.0.1 - Initial Release

### Added
- **Core Functionality**: Distributed hash cracking coordination
- **Worker Communication**: RESTful API for worker nodes
- **Basic Web Interface**: Job creation and monitoring
- **SQLite Support**: Local database for development and small deployments

---

## Notes

- This changelog follows [Keep a Changelog](https://keepachangelog.com/) format
- Version numbers follow [Semantic Versioning](https://semver.org/)
- Dates are in YYYY-MM-DD format
- This file is automatically loaded by HashTasker - no server restart required for updates