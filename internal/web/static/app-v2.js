// Spot Analyzer v2 - Modern Dashboard JavaScript

// State management
const state = {
    architecture: 'any',
    selectedFamilies: [],
    availableFamilies: [],
    results: [],
    sortColumn: 'score',
    sortDirection: 'desc'
};

// DOM Ready
document.addEventListener('DOMContentLoaded', () => {
    initTheme();
    initNavigation();
    initArchButtons();
    initStabilitySlider();
    loadPresets();
    loadFamilies();
    loadCacheStatus();
    bindEventListeners();
});

// Theme Management
function initTheme() {
    const savedTheme = localStorage.getItem('spot-analyzer-theme') || 'light';
    document.documentElement.setAttribute('data-theme', savedTheme);
    
    document.getElementById('themeToggle').addEventListener('click', () => {
        const current = document.documentElement.getAttribute('data-theme');
        const next = current === 'light' ? 'dark' : 'light';
        document.documentElement.setAttribute('data-theme', next);
        localStorage.setItem('spot-analyzer-theme', next);
    });
}

// Navigation
function initNavigation() {
    const navItems = document.querySelectorAll('.nav-item[data-section]');
    navItems.forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            const section = item.dataset.section;
            
            // Update nav
            navItems.forEach(n => n.classList.remove('active'));
            item.classList.add('active');
            
            // Update page title
            const titles = {
                'analyze': 'Instance Analysis',
                'az-lookup': 'AZ Lookup'
            };
            document.getElementById('pageTitle').textContent = titles[section] || section;
            
            // Show section
            document.querySelectorAll('.section').forEach(s => {
                s.classList.remove('active');
                s.classList.add('hidden');
            });
            // Map section names to element IDs
            const sectionIds = {
                'analyze': 'analyzeSection',
                'az-lookup': 'azLookupSection'
            };
            const sectionId = sectionIds[section] || (section + 'Section');
            const sectionEl = document.getElementById(sectionId);
            if (sectionEl) {
                sectionEl.classList.add('active');
                sectionEl.classList.remove('hidden');
            }
        });
    });
}

// Architecture buttons
function initArchButtons() {
    const archBtns = document.querySelectorAll('.arch-btn');
    archBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            archBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.architecture = btn.dataset.arch;
        });
    });
}

// Stability slider
function initStabilitySlider() {
    const slider = document.getElementById('interruption');
    const labels = document.querySelectorAll('.stability-label');
    
    function updateLabels() {
        const value = parseInt(slider.value);
        labels.forEach(label => {
            label.classList.toggle('active', parseInt(label.dataset.value) === value);
        });
    }
    
    slider.addEventListener('input', updateLabels);
    updateLabels();
}

// Event Listeners
function bindEventListeners() {
    // Parse button
    document.getElementById('parseBtn').addEventListener('click', parseRequirements);
    
    // Analyze button
    document.getElementById('analyzeBtn').addEventListener('click', analyzeInstances);
    
    // Refresh cache
    document.getElementById('refreshCacheBtn').addEventListener('click', refreshCache);
    
    // AZ Lookup
    const azLookupBtn = document.getElementById('azLookupBtn');
    if (azLookupBtn) {
        azLookupBtn.addEventListener('click', lookupAZ);
    }
    
    // Table search
    const tableSearch = document.getElementById('tableSearch');
    if (tableSearch) {
        tableSearch.addEventListener('input', filterTable);
    }
}

// Load Presets
async function loadPresets() {
    try {
        const response = await fetch('/api/presets');
        const presets = await response.json();
        
        const grid = document.getElementById('presetsGrid');
        grid.innerHTML = presets.map(preset => `
            <button class="preset-btn" data-preset='${JSON.stringify(preset)}'>
                <span>${preset.icon || '📦'}</span>
                <span>${preset.name}</span>
            </button>
        `).join('');
        
        // Bind preset clicks
        grid.querySelectorAll('.preset-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const preset = JSON.parse(btn.dataset.preset);
                applyPreset(preset);
            });
        });
    } catch (error) {
        console.error('Failed to load presets:', error);
    }
}

// Apply Preset
function applyPreset(preset) {
    if (preset.minVcpu) document.getElementById('minVcpu').value = preset.minVcpu;
    if (preset.maxVcpu) document.getElementById('maxVcpu').value = preset.maxVcpu;
    if (preset.minMemory) document.getElementById('minMemory').value = preset.minMemory;
    if (preset.maxMemory) document.getElementById('maxMemory').value = preset.maxMemory;
    
    if (preset.architecture) {
        state.architecture = preset.architecture;
        document.querySelectorAll('.arch-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.arch === preset.architecture);
        });
    }
    
    if (preset.maxInterruption !== undefined) {
        const slider = document.getElementById('interruption');
        slider.value = preset.maxInterruption;
        slider.dispatchEvent(new Event('input'));
    }
}

// Load Families
async function loadFamilies() {
    try {
        const response = await fetch('/api/families');
        const families = await response.json();
        
        state.availableFamilies = families || [];
        
        const container = document.getElementById('familyChips');
        container.innerHTML = families.map(f => `
            <button class="family-chip" data-family="${f.Name || f.name}">
                <span class="family-chip-name">${f.Name || f.name}</span>
                <span class="family-chip-desc">${f.Description || f.description || ''}</span>
            </button>
        `).join('');
        
        // Bind family clicks
        container.querySelectorAll('.family-chip').forEach(chip => {
            chip.addEventListener('click', () => {
                chip.classList.toggle('active');
                updateSelectedFamilies();
            });
        });
    } catch (error) {
        console.error('Failed to load families:', error);
    }
}

function updateSelectedFamilies() {
    const chips = document.querySelectorAll('.family-chip.active');
    state.selectedFamilies = Array.from(chips).map(c => c.dataset.family);
    
    const badge = document.getElementById('familyCount');
    badge.textContent = state.selectedFamilies.length > 0 
        ? state.selectedFamilies.length 
        : 'All';
}

// Cache Status
async function loadCacheStatus() {
    try {
        const response = await fetch('/api/cache/status');
        const data = await response.json();
        
        const badge = document.getElementById('cacheStatusBadge');
        const text = document.getElementById('cacheStatusText');
        
        if (data.items > 0) {
            const hitRate = data.hits + data.misses > 0 
                ? Math.round((data.hits / (data.hits + data.misses)) * 100) 
                : 0;
            text.textContent = `Cached (${data.items} items, ${hitRate}% hit)`;
            badge.classList.remove('stale');
        } else {
            text.textContent = 'No Cache';
            badge.classList.add('stale');
        }
    } catch (error) {
        document.getElementById('cacheStatusText').textContent = 'Error';
    }
}

// Refresh Cache
async function refreshCache() {
    const btn = document.getElementById('refreshCacheBtn');
    btn.disabled = true;
    btn.innerHTML = '<span>⏳</span><span>Refreshing...</span>';
    
    try {
        await fetch('/api/cache/refresh', { method: 'POST' });
        await loadCacheStatus();
        btn.innerHTML = '<span>✅</span><span>Refreshed!</span>';
        setTimeout(() => {
            btn.innerHTML = '<span>🔄</span><span>Refresh Data</span>';
            btn.disabled = false;
        }, 2000);
    } catch (error) {
        btn.innerHTML = '<span>❌</span><span>Error</span>';
        btn.disabled = false;
    }
}

// Parse Requirements
async function parseRequirements() {
    const input = document.getElementById('nlInput').value.trim();
    if (!input) return;
    
    const resultDiv = document.getElementById('parseResult');
    resultDiv.classList.remove('hidden');
    resultDiv.innerHTML = '<p>🔄 Parsing...</p>';
    
    try {
        const response = await fetch('/api/parse-requirements', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ text: input })
        });
        
        const data = await response.json();
        
        // Server returns config directly, not nested under 'config'
        if (data.minVcpu !== undefined || data.minMemory !== undefined || data.explanation) {
            // Map server response to preset format
            const config = {
                minVcpu: data.minVcpu,
                maxVcpu: data.maxVcpu,
                minMemory: data.minMemory,
                maxMemory: data.maxMemory,
                architecture: data.architecture,
                maxInterruption: data.maxInterruption,
                useCase: data.useCase
            };
            applyPreset(config);
            resultDiv.innerHTML = `
                <div class="parse-success">
                    <h4>✅ ${data.explanation || 'Parsed Configuration'}</h4>
                    <p>vCPU: ${data.minVcpu}${data.maxVcpu ? '-' + data.maxVcpu : '+'} | Memory: ${data.minMemory}${data.maxMemory ? '-' + data.maxMemory : '+'}GB${data.architecture ? ' | Arch: ' + data.architecture : ''}${data.useCase ? ' | Use Case: ' + data.useCase : ''}</p>
                </div>
            `;
        } else {
            resultDiv.innerHTML = '<p>❌ Could not parse requirements</p>';
        }
    } catch (error) {
        resultDiv.innerHTML = `<p>❌ Error: ${error.message}</p>`;
    }
}

// Analyze Instances
async function analyzeInstances() {
    const loading = document.getElementById('loading');
    const results = document.getElementById('results');
    
    loading.classList.remove('hidden');
    results.classList.add('hidden');
    
    const request = {
        minVcpu: parseInt(document.getElementById('minVcpu').value) || 1,
        maxVcpu: parseInt(document.getElementById('maxVcpu').value) || 0,
        minMemory: parseFloat(document.getElementById('minMemory').value) || 0,
        maxMemory: parseFloat(document.getElementById('maxMemory').value) || 0,
        region: document.getElementById('region').value,
        architecture: state.architecture,
        maxInterruption: parseInt(document.getElementById('interruption').value),
        enhanced: document.getElementById('enhanced').checked,
        topN: parseInt(document.getElementById('topN').value) || 15,
        families: state.selectedFamilies.length > 0 ? state.selectedFamilies : undefined,
        refreshCache: document.getElementById('refreshCache')?.checked || false
    };
    
    try {
        const response = await fetch('/api/analyze', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(request)
        });
        
        const data = await response.json();
        
        if (data.error) {
            loading.classList.add('hidden');
            alert('Error: ' + data.error);
            return;
        }
        
        state.results = data.instances || [];
        displayResults(data);
        
    } catch (error) {
        loading.classList.add('hidden');
        alert('Analysis failed: ' + error.message);
    }
}

// Display Results
function displayResults(data) {
    const loading = document.getElementById('loading');
    const results = document.getElementById('results');
    
    loading.classList.add('hidden');
    results.classList.remove('hidden');
    
    // Update stats
    const instances = data.instances || [];
    document.getElementById('statTotal').textContent = instances.length;
    
    if (instances.length > 0) {
        const avgSavings = instances.reduce((sum, r) => sum + (r.savingsPercent || 0), 0) / instances.length;
        document.getElementById('statSavings').textContent = avgSavings.toFixed(0) + '%';
        document.getElementById('statBest').textContent = instances[0].instanceType;
        document.getElementById('statBestAZ').textContent = '⏳ Loading...';
    }
    
    // Update freshness
    document.getElementById('dataSourceValue').textContent = data.dataSource || 'AWS API';
    document.getElementById('freshnessStatus').textContent = data.cachedData ? 'Cached (2h TTL)' : 'Fresh fetch';
    document.getElementById('analyzedAt').textContent = new Date().toLocaleTimeString();
    
    // Update insights
    if (data.insights && data.insights.length > 0) {
        document.getElementById('insights').innerHTML = data.insights.map(insight => {
            // Handle both string and object insights
            const text = typeof insight === 'string' ? insight : (insight.description || insight.title || '');
            const icon = typeof insight === 'object' && insight.type === 'best' ? '🏆' : '💡';
            return `
            <div class="insight-card">
                <span class="insight-icon">${icon}</span>
                <div class="insight-content">
                    <p>${text}</p>
                </div>
            </div>
        `;
        }).join('');
    }
    
    // Update table
    renderResultsTable(data.instances || []);
}

// Render Results Table
function renderResultsTable(instances) {
    const tbody = document.getElementById('resultsBody');
    const region = document.getElementById('region').value;
    
    tbody.innerHTML = instances.map((r, i) => `
        <tr>
            <td>${r.rank || i + 1}</td>
            <td><span class="instance-name">${r.instanceType}</span></td>
            <td>${r.vcpu}</td>
            <td>${(r.memoryGb || 0).toFixed(1)} GB</td>
            <td><span class="savings-badge">${r.savingsPercent || 0}%</span></td>
            <td>${r.interruptionLevel || '-'}</td>
            <td><span class="score-badge">${(r.score || 0).toFixed(2)}</span></td>
            <td><span class="arch-badge">${r.architecture || '-'}</span></td>
            <td>
                <span class="az-cell" data-instance="${r.instanceType}" data-region="${region}" title="Click for top 3 AZs">
                    <span class="az-value">⏳</span>
                </span>
            </td>
        </tr>
    `).join('');
    
    // Auto-fetch Best AZ for all rows
    autoFetchAllAZs(tbody, region);
}

// Auto-fetch Best AZ for all instances
async function autoFetchAllAZs(tbody, region) {
    const azCells = Array.from(tbody.querySelectorAll('.az-cell'));
    if (azCells.length === 0) return;
    
    // First, fetch the #1 ranked instance's AZ and update stat card
    const firstAz = await fetchAZForCell(azCells[0]);
    if (firstAz) {
        document.getElementById('statBestAZ').textContent = firstAz;
    }
    
    // Then fetch the rest in batches
    const remaining = azCells.slice(1);
    const batchSize = 5;
    for (let i = 0; i < remaining.length; i += batchSize) {
        const batch = remaining.slice(i, i + batchSize);
        await Promise.all(batch.map(cell => fetchAZForCell(cell)));
    }
    
    // Refresh cache status after all AZ fetches complete
    loadCacheStatus();
}

async function fetchAZForCell(cell) {
    const instanceType = cell.dataset.instance;
    const region = cell.dataset.region;
    const valueSpan = cell.querySelector('.az-value');
    
    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ instanceType, region })
        });
        const data = await response.json();
        
        if (data.bestAz) {
            valueSpan.innerHTML = `<strong class="az-link">${data.bestAz}</strong>`;
            cell.style.cursor = 'pointer';
            cell.onclick = () => showAZDetails(instanceType, region);
            return data.bestAz;
        } else {
            valueSpan.textContent = 'N/A';
            return null;
        }
    } catch (e) {
        valueSpan.textContent = '❌';
        return null;
    }
}

function getInterruptionLabel(rate) {
    if (rate === 0) return '<5%';
    if (rate === 1) return '5-10%';
    if (rate === 2) return '10-15%';
    if (rate === 3) return '15-20%';
    return '>20%';
}

// Filter Table
function filterTable() {
    const search = document.getElementById('tableSearch').value.toLowerCase();
    const rows = document.querySelectorAll('#resultsBody tr');
    
    rows.forEach(row => {
        const text = row.textContent.toLowerCase();
        row.style.display = text.includes(search) ? '' : 'none';
    });
}

// AZ Details
async function showAZDetails(instanceType, region) {
    const modal = document.getElementById('azModal');
    const loading = document.getElementById('modalAzLoading');
    const content = document.getElementById('modalAzContent');
    
    modal.classList.remove('hidden');
    loading.classList.remove('hidden');
    content.classList.add('hidden');
    
    document.getElementById('modalInstanceType').textContent = instanceType;
    
    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ instanceType, region })
        });
        
        const data = await response.json();
        
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        
        // Render insights
        const insightsDiv = document.getElementById('modalAzInsights');
        if (data.bestAz) {
            insightsDiv.innerHTML = `
                <div class="insight-card">
                    <span class="insight-icon">🏆</span>
                    <div class="insight-content">
                        <h4>Best AZ: ${data.bestAz}</h4>
                        <p>Lowest average price and best stability in ${region}</p>
                    </div>
                </div>
                ${data.nextBestAz ? `
                <div class="insight-card">
                    <span class="insight-icon">🥈</span>
                    <div class="insight-content">
                        <h4>Second Best: ${data.nextBestAz}</h4>
                        <p>Good alternative for failover or capacity</p>
                    </div>
                </div>
                ` : ''}
            `;
        }
        
        // Render table
        const tbody = document.getElementById('modalAzBody');
        tbody.innerHTML = (data.recommendations || []).map((az, i) => `
            <tr>
                <td>${az.rank || i + 1}</td>
                <td><strong>${az.availabilityZone}</strong></td>
                <td>$${az.avgPrice.toFixed(4)}</td>
                <td>$${az.currentPrice.toFixed(4)}</td>
                <td>$${az.minPrice.toFixed(4)}</td>
                <td>$${az.maxPrice.toFixed(4)}</td>
                <td>${az.stability}</td>
            </tr>
        `).join('');
        
    } catch (error) {
        loading.innerHTML = `<p>❌ Error: ${error.message}</p>`;
    }
}

function closeAZModal() {
    document.getElementById('azModal').classList.add('hidden');
}

// AZ Lookup (standalone)
async function lookupAZ() {
    const instanceType = document.getElementById('azInstanceType').value.trim();
    const region = document.getElementById('azRegion').value;
    
    if (!instanceType) {
        alert('Please enter an instance type');
        return;
    }
    
    const loading = document.getElementById('azLoading');
    const results = document.getElementById('azResults');
    
    loading.classList.remove('hidden');
    results.classList.add('hidden');
    
    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ instanceType, region })
        });
        
        const data = await response.json();
        
        loading.classList.add('hidden');
        results.classList.remove('hidden');
        
        // Render results similar to modal
        results.innerHTML = `
            <div class="card">
                <div class="card-header">
                    <h3>${instanceType} in ${region}</h3>
                </div>
                <div class="card-body">
                    ${data.bestAz ? `<p><strong>Best AZ:</strong> ${data.bestAz}</p>` : ''}
                    ${data.nextBestAz ? `<p><strong>Next Best:</strong> ${data.nextBestAz}</p>` : ''}
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>Rank</th>
                                <th>AZ</th>
                                <th>Avg Price</th>
                                <th>Current</th>
                                <th>Min</th>
                                <th>Max</th>
                                <th>Stability</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${(data.recommendations || []).map(az => `
                                <tr>
                                    <td>${az.rank}</td>
                                    <td><strong>${az.availabilityZone}</strong></td>
                                    <td>$${az.avgPrice.toFixed(4)}</td>
                                    <td>$${az.currentPrice.toFixed(4)}</td>
                                    <td>$${az.minPrice.toFixed(4)}</td>
                                    <td>$${az.maxPrice.toFixed(4)}</td>
                                    <td>${az.stability}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
            </div>
        `;
        
    } catch (error) {
        loading.classList.add('hidden');
        results.classList.remove('hidden');
        results.innerHTML = `<div class="card"><div class="card-body"><p>❌ Error: ${error.message}</p></div></div>`;
    }
}

// Keyboard shortcuts
document.addEventListener('keydown', (e) => {
    // Ctrl+Enter to analyze
    if (e.ctrlKey && e.key === 'Enter') {
        analyzeInstances();
    }
    // Escape to close modal
    if (e.key === 'Escape') {
        closeAZModal();
    }
});
