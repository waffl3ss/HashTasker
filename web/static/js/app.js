// HashTasker JavaScript utilities

// Global app configuration
const HashTasker = {
    config: {
        refreshInterval: 30000, // 30 seconds
        apiBase: '/api'
    }
};

// Utility functions
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatDuration(seconds) {
    if (seconds < 60) {
        return Math.round(seconds) + 's';
    } else if (seconds < 3600) {
        return Math.round(seconds / 60) + 'm';
    } else if (seconds < 86400) {
        return Math.round(seconds / 3600) + 'h';
    } else {
        return Math.round(seconds / 86400) + 'd';
    }
}

function formatDateTime(date) {
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
}

// API helpers
async function apiCall(endpoint, options = {}) {
    const url = HashTasker.config.apiBase + endpoint;
    const defaultOptions = {
        headers: {
            'Content-Type': 'application/json',
        },
    };
    
    const response = await fetch(url, { ...defaultOptions, ...options });
    
    if (!response.ok) {
        throw new Error(`API call failed: ${response.status} ${response.statusText}`);
    }
    
    return response.json();
}

// Toast notifications
function showToast(message, type = 'info') {
    // Create toast container if it doesn't exist
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        container.className = 'position-fixed top-0 end-0 p-3';
        container.style.zIndex = '1050';
        document.body.appendChild(container);
    }
    
    const toast = document.createElement('div');
    toast.className = `toast align-items-center text-white bg-${type} border-0`;
    toast.setAttribute('role', 'alert');
    toast.innerHTML = `
        <div class="d-flex">
            <div class="toast-body">${message}</div>
            <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button>
        </div>
    `;
    
    container.appendChild(toast);
    
    const bsToast = new bootstrap.Toast(toast);
    bsToast.show();
    
    // Remove toast element after it's hidden
    toast.addEventListener('hidden.bs.toast', () => {
        container.removeChild(toast);
    });
}

// Loading states
function setLoading(element, isLoading) {
    if (isLoading) {
        element.disabled = true;
        const originalText = element.textContent;
        element.setAttribute('data-original-text', originalText);
        element.innerHTML = '<div class="loading-spinner me-2"></div>Loading...';
    } else {
        element.disabled = false;
        const originalText = element.getAttribute('data-original-text');
        element.innerHTML = originalText;
        element.removeAttribute('data-original-text');
    }
}

// Confirmation dialogs
function confirmDialog(message, callback) {
    if (confirm(message)) {
        callback();
    }
}

// Status badge helpers
function getStatusBadge(status) {
    const badges = {
        'queued': '<span class="badge bg-secondary">Queued</span>',
        'running': '<span class="badge bg-primary">Running</span>',
        'completed': '<span class="badge bg-success">Completed</span>',
        'cancelled': '<span class="badge bg-warning">Cancelled</span>',
        'failed': '<span class="badge bg-danger">Failed</span>',
        'online': '<span class="badge bg-success">Online</span>',
        'offline': '<span class="badge bg-danger">Offline</span>'
    };
    return badges[status.toLowerCase()] || '<span class="badge bg-secondary">Unknown</span>';
}

// Progress bar helpers
function createProgressBar(percentage, animated = false, variant = 'primary') {
    const animatedClass = animated ? 'progress-bar-animated' : '';
    const stripedClass = animated ? 'progress-bar-striped' : '';
    
    return `
        <div class="progress">
            <div class="progress-bar bg-${variant} ${stripedClass} ${animatedClass}" 
                 style="width: ${percentage}%" 
                 role="progressbar" 
                 aria-valuenow="${percentage}" 
                 aria-valuemin="0" 
                 aria-valuemax="100">
                ${percentage.toFixed(1)}%
            </div>
        </div>
    `;
}

// Auto-refresh functionality
class AutoRefresh {
    constructor(callback, interval = 30000) {
        this.callback = callback;
        this.interval = interval;
        this.intervalId = null;
        this.isEnabled = true;
    }
    
    start() {
        if (this.isEnabled && !this.intervalId) {
            this.intervalId = setInterval(this.callback, this.interval);
        }
    }
    
    stop() {
        if (this.intervalId) {
            clearInterval(this.intervalId);
            this.intervalId = null;
        }
    }
    
    toggle() {
        this.isEnabled = !this.isEnabled;
        if (this.isEnabled) {
            this.start();
        } else {
            this.stop();
        }
        return this.isEnabled;
    }
    
    setInterval(newInterval) {
        this.interval = newInterval;
        if (this.intervalId) {
            this.stop();
            this.start();
        }
    }
}

// Search and filter helpers
function filterTable(tableId, searchTerm, columnIndices = []) {
    const table = document.getElementById(tableId);
    const tbody = table.querySelector('tbody');
    const rows = tbody.querySelectorAll('tr');
    
    const term = searchTerm.toLowerCase();
    
    rows.forEach(row => {
        let shouldShow = false;
        
        if (columnIndices.length === 0) {
            // Search all columns
            const text = row.textContent.toLowerCase();
            shouldShow = text.includes(term);
        } else {
            // Search specific columns
            columnIndices.forEach(index => {
                const cell = row.cells[index];
                if (cell && cell.textContent.toLowerCase().includes(term)) {
                    shouldShow = true;
                }
            });
        }
        
        row.style.display = shouldShow ? '' : 'none';
    });
}

// Copy to clipboard
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('Copied to clipboard', 'success');
        return true;
    } catch (err) {
        console.error('Failed to copy to clipboard:', err);
        showToast('Failed to copy to clipboard', 'danger');
        return false;
    }
}

// Download file
function downloadFile(data, filename, mimeType = 'text/plain') {
    const blob = new Blob([data], { type: mimeType });
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    window.URL.revokeObjectURL(url);
}

// Initialize common functionality when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Add tooltips to all elements with title attribute
    const tooltipTriggerList = [].slice.call(document.querySelectorAll('[title]'));
    tooltipTriggerList.map(function (tooltipTriggerEl) {
        return new bootstrap.Tooltip(tooltipTriggerEl);
    });
    
    // Initialize popovers
    const popoverTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="popover"]'));
    popoverTriggerList.map(function (popoverTriggerEl) {
        return new bootstrap.Popover(popoverTriggerEl);
    });
});