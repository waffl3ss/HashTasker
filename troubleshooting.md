# HashTasker Troubleshooting Guide

This guide covers common issues and solutions for HashTasker setup and operation.

## Common Issues

### Workers Not Connecting

**Problem:** Workers are not appearing in the dashboard or not receiving jobs.

**Solutions:**
1. **Check Network Connectivity**
   - Ensure workers can reach the server on the configured port (default: 8080)
   - Test with: `curl http://your-server:8080/api/worker/stats`

2. **Verify Worker Configuration**
   - Check `worker.json` has correct `server_url`
   - Ensure `hostname` is unique for each worker
   - Verify `checkin_interval` is reasonable (e.g., "30s")

3. **Check Server Logs**
   - Look for worker registration messages
   - Check for authentication or network errors

### Jobs Stuck in "Queued" Status

**Problem:** Jobs remain in queued status and never start.

**Solutions:**
1. **Verify Active Workers**
   - Check dashboard for online workers
   - Ensure workers have available capacity

2. **Check Job Configuration**
   - Verify wordlist file exists and is accessible
   - Ensure hash mode is valid
   - Check ruleset files are present on workers

3. **Worker Capacity Issues**
   - Workers may be overloaded with existing jobs
   - Check worker process limits in configuration

### Hash Mode Issues

**Problem:** Jobs fail with invalid hash mode errors.

**Solutions:**
1. **Verify Hash Mode**
   - Check `hashmodes.txt` file is present
   - Ensure selected mode matches your hash format
   - Test with known working hash examples

2. **Update Hash Modes**
   - Ensure `hashmodes.txt` is up to date
   - Restart server after updating hash modes file

### File Access Problems

**Problem:** Workers can't access wordlists or rules files.

**Solutions:**
1. **Check File Permissions**
   - Ensure worker process can read wordlist files
   - Verify ruleset files have correct permissions

2. **Path Configuration**
   - Check `wordlists_path` in server config
   - Verify `rulesets_path` matches between server and workers
   - Use absolute paths when possible

3. **File Synchronization**
   - Ensure ruleset files exist on all worker nodes
   - Maintain identical directory structure across workers

### Database Connection Issues

**Problem:** Server fails to start with database errors.

**Solutions:**
1. **SQLite Issues (Default)**
   - Check disk space and permissions
   - Ensure directory is writable
   - Try deleting `hashtasker.db` for fresh start

2. **MySQL Configuration**
   - Verify connection details in `config.yaml`
   - Check MySQL server is running and accessible
   - Ensure database exists and user has permissions

### Authentication Problems

**Problem:** LDAP authentication not working or login failures.

**Solutions:**
1. **LDAP Configuration**
   - Verify LDAP server connection details
   - Check bind DN and password
   - Test LDAP connectivity separately

2. **Local Admin Access**
   - Use local admin account for initial setup
   - Reset admin password in `config.yaml`

3. **Session Issues**
   - Clear browser cookies and cache
   - Check session key configuration

## Performance Optimization

### Worker Performance

1. **Resource Allocation**
   - Ensure adequate CPU and memory for workers
   - Monitor GPU utilization if using GPU acceleration
   - Balance worker load across available hardware

2. **Concurrent Jobs**
   - Adjust maximum concurrent hashcat processes
   - Monitor system resources under load
   - Consider worker specialization for different job types

### Server Performance

1. **Database Optimization**
   - Regular database maintenance
   - Monitor query performance
   - Consider MySQL for large deployments

2. **File I/O Optimization**
   - Use fast storage for wordlists and temporary files
   - Consider network storage for shared wordlists
   - Monitor disk space usage

## Configuration Best Practices

### Security

1. **Session Management**
   - Use strong, unique session keys
   - Regular key rotation
   - Secure cookie settings

2. **Network Security**
   - Use HTTPS in production
   - Implement proper firewall rules
   - Consider VPN for worker communications

3. **Access Control**
   - Use LDAP for centralized authentication
   - Regular user access reviews
   - Implement proper admin controls

### Monitoring

1. **System Monitoring**
   - Monitor worker health and connectivity
   - Track job completion rates
   - Monitor resource utilization

2. **Log Management**
   - Regular log rotation
   - Monitor for error patterns
   - Implement alerting for critical issues

## Getting Help

If you continue to experience issues:

1. **Check Logs**
   - Server logs for system-level issues
   - Worker logs for job-specific problems
   - Browser console for UI issues

2. **Gather Information**
   - System specifications
   - Configuration files (sanitized)
   - Error messages and logs
   - Steps to reproduce the issue

3. **Community Support**
   - GitHub Issues: Report bugs and feature requests
   - Check existing issues for similar problems
   - Provide detailed problem descriptions

Remember to sanitize any sensitive information (passwords, hashes, internal hostnames) before sharing logs or configurations.