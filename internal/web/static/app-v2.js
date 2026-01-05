// Spot Analyzer v2 - Modern Dashboard JavaScript

// State management
const state = {
    cloudProvider: 'aws',
    azCloudProvider: 'aws',  // Separate state for AZ Lookup
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
    initCloudButtons();
    initAZCloudButtons();  // Initialize AZ Lookup cloud buttons
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

// Cloud provider buttons (Analyze tab)
function initCloudButtons() {
    const cloudBtns = document.querySelectorAll('.cloud-btn');
    cloudBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            cloudBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.cloudProvider = btn.dataset.cloud;
            updateRegionsForCloud(btn.dataset.cloud);
            // Reload families for the selected cloud
            loadFamilies();
        });
    });
}

// Cloud provider buttons (AZ Lookup tab - independent from Analyze tab)
function initAZCloudButtons() {
    const azCloudBtns = document.querySelectorAll('.az-cloud-btn');
    azCloudBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            azCloudBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.azCloudProvider = btn.dataset.cloud;
            updateAZRegionsForCloud(btn.dataset.cloud);
            updateAZInstanceHint(btn.dataset.cloud);
        });
    });
}

// Update instance type hint based on selected cloud
function updateAZInstanceHint(cloud) {
    const hint = document.getElementById('azInstanceHint');
    if (!hint) return;
    
    const hints = {
        aws: 'AWS: "m5", "c6i", "m5.large", "c6i.xlarge"',
        azure: 'Azure: "Standard_D", "Standard_E", "Standard_D2s_v5"',
        gcp: 'GCP: "n2-standard", "e2-medium", "n2-standard-4"'
    };
    hint.textContent = hints[cloud] || hints.aws;
}

// Update region dropdown based on selected cloud provider
function updateRegionsForCloud(cloud) {
    const regionSelect = document.getElementById('region');

    // Get all optgroups
    const awsOptgroups = document.querySelectorAll('[id^="aws-"]');
    const azureOptgroups = document.querySelectorAll('[id^="azure-"]');
    const gcpOptgroups = document.querySelectorAll('[id^="gcp-"]');

    if (cloud === 'azure') {
        // Hide AWS and GCP, show Azure
        awsOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.add('hidden'));
        azureOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default Azure region
        regionSelect.value = 'eastus';
    } else if (cloud === 'gcp') {
        // Hide AWS and Azure, show GCP
        awsOptgroups.forEach(og => og.classList.add('hidden'));
        azureOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default GCP region
        regionSelect.value = 'us-central1';
    } else {
        // Hide Azure and GCP, show AWS
        azureOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.add('hidden'));
        awsOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default AWS region
        regionSelect.value = 'us-east-1';
    }
}

// Update AZ Lookup region dropdown based on selected cloud provider
function updateAZRegionsForCloud(cloud) {
    const azRegionSelect = document.getElementById('azRegion');
    if (!azRegionSelect) return;

    // Get all optgroups for AZ lookup
    const awsOptgroups = document.querySelectorAll('[id^="az-aws-"]');
    const azureOptgroups = document.querySelectorAll('[id^="az-azure-"]');
    const gcpOptgroups = document.querySelectorAll('[id^="az-gcp-"]');

    if (cloud === 'azure') {
        // Hide AWS and GCP, show Azure
        awsOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.add('hidden'));
        azureOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default Azure region
        azRegionSelect.value = 'eastus';
    } else if (cloud === 'gcp') {
        // Hide AWS and Azure, show GCP
        awsOptgroups.forEach(og => og.classList.add('hidden'));
        azureOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default GCP region
        azRegionSelect.value = 'us-central1';
    } else {
        // Hide Azure and GCP, show AWS
        azureOptgroups.forEach(og => og.classList.add('hidden'));
        gcpOptgroups.forEach(og => og.classList.add('hidden'));
        awsOptgroups.forEach(og => og.classList.remove('hidden'));
        // Set default AWS region
        azRegionSelect.value = 'us-east-1';
    }
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
    
    // Instance type autocomplete
    const azInstanceInput = document.getElementById('azInstanceType');
    if (azInstanceInput) {
        azInstanceInput.addEventListener('input', debounce(handleInstanceTypeInput, 300));
        azInstanceInput.addEventListener('focus', () => {
            if (azInstanceInput.value.length >= 1) {
                handleInstanceTypeInput();
            }
        });
        azInstanceInput.addEventListener('keydown', handleAutocompleteKeydown);

        // Close dropdown when clicking outside
        document.addEventListener('click', (e) => {
            const container = document.querySelector('.autocomplete-container');
            if (container && !container.contains(e.target)) {
                hideAutocompleteDropdown();
            }
        });
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
        // Pass cloud provider to get appropriate families
        const response = await fetch(`/api/families?cloud=${state.cloudProvider}`);
        const families = await response.json();
        
        state.availableFamilies = families || [];
        state.selectedFamilies = []; // Reset selection when switching clouds

        const container = document.getElementById('familyChips');
        container.innerHTML = families.map(f => `
            <button class="family-chip" data-family="${f.Name || f.name}">
                <span class="family-chip-name">${f.Name || f.name}</span>
                <span class="family-chip-desc">${f.Description || f.description || ''}</span>
            </button>
        `).join('');
        
        // Update badge
        document.getElementById('familyCount').textContent = 'All';

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
        cloudProvider: state.cloudProvider,
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
    } else {
        // Reset stats when no instances found
        document.getElementById('statSavings').textContent = '-';
        document.getElementById('statBest').textContent = '-';
        document.getElementById('statBestAZ').textContent = '-';
    }
    
    // Update freshness
    document.getElementById('dataSourceValue').textContent = data.dataSource || 'AWS API';
    document.getElementById('freshnessStatus').textContent = data.cachedData ? 'Cached (2h TTL)' : 'Fresh fetch';
    document.getElementById('analyzedAt').textContent = new Date().toLocaleTimeString();
    
    // Update insights
    if (instances.length > 0 && data.insights && data.insights.length > 0) {
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
    } else {
        // Clear insights when no instances
        document.getElementById('insights').innerHTML = '<div class="insight-card"><span class="insight-icon">ℹ️</span><div class="insight-content"><p>No instances match your criteria. Try adjusting filters.</p></div></div>';
    }
    
    // Update table
    renderResultsTable(data.instances || []);
}

// Render Results Table
function renderResultsTable(instances) {
    const tbody = document.getElementById('resultsBody');
    const region = document.getElementById('region').value;
    const cloudProvider = state.cloudProvider; // Get current cloud provider

    if (instances.length === 0) {
        tbody.innerHTML = '<tr><td colspan="9" style="text-align: center; padding: 2rem; color: var(--text-secondary);">No instances found. Try adjusting your filters.</td></tr>';
        return;
    }

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
                <span class="az-cell" data-instance="${r.instanceType}" data-region="${region}" data-cloud="${cloudProvider}" title="Click for top 3 AZs">
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
    const cloudProvider = cell.dataset.cloud || state.cloudProvider; // Get cloud provider
    const valueSpan = cell.querySelector('.az-value');

    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                cloudProvider: cloudProvider,
                instanceType: instanceType,
                region: region
            })
        });
        const data = await response.json();
        
        if (data.bestAz) {
            // Show best AZ with score if available
            const score = data.recommendations?.[0]?.combinedScore;
            const scoreText = score ? ` <small style="opacity:0.7">(${score.toFixed(0)})</small>` : '';
            valueSpan.innerHTML = `<strong class="az-link">${data.bestAz}</strong>${scoreText}`;
            cell.style.cursor = 'pointer';
            cell.onclick = () => showAZDetails(instanceType, region, cloudProvider);
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
async function showAZDetails(instanceType, region, cloudProvider) {
    const modal = document.getElementById('azModal');
    const loading = document.getElementById('modalAzLoading');
    const content = document.getElementById('modalAzContent');
    
    // Use passed cloudProvider or fall back to state
    const cloud = cloudProvider || state.cloudProvider;

    modal.classList.remove('hidden');
    loading.classList.remove('hidden');
    content.classList.add('hidden');
    
    document.getElementById('modalInstanceType').textContent = instanceType;
    
    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                cloudProvider: cloud,
                instanceType: instanceType,
                region: region
            })
        });
        
        const data = await response.json();
        
        loading.classList.add('hidden');
        content.classList.remove('hidden');
        
        // Render insights
        const insightsDiv = document.getElementById('modalAzInsights');
        if (data.bestAz) {
            // Get scores from recommendations
            const bestRec = data.recommendations?.find(r => r.availabilityZone === data.bestAz);
            const nextRec = data.recommendations?.find(r => r.availabilityZone === data.nextBestAz);
            const bestScore = bestRec?.combinedScore || bestRec?.score || 0;
            const nextScore = nextRec?.combinedScore || nextRec?.score || 0;
            
            insightsDiv.innerHTML = `
                <div class="insight-card">
                    <span class="insight-icon">🏆</span>
                    <div class="insight-content">
                        <h4>Best AZ: ${data.bestAz} <span style="color: var(--primary-color); font-weight: bold;">(${bestScore.toFixed(0)})</span></h4>
                        <p>Lowest average price and best stability in ${region}</p>
                    </div>
                </div>
                ${data.nextBestAz ? `
                <div class="insight-card">
                    <span class="insight-icon">🥈</span>
                    <div class="insight-content">
                        <h4>Second Best: ${data.nextBestAz} <span style="color: var(--secondary-color); font-weight: bold;">(${nextScore.toFixed(0)})</span></h4>
                        <p>Good alternative for failover or capacity</p>
                    </div>
                </div>
                ` : ''}
            `;
        }
        
        // Render table
        const tbody = document.getElementById('modalAzBody');
        tbody.innerHTML = (data.recommendations || []).map((az, i) => {
            const score = az.combinedScore || az.score || 0;
            const capacityScore = az.capacityScore || 0;
            const capacityLevel = az.capacityLevel || 'medium';
            const capacityClass = capacityLevel.toLowerCase() === 'high' ? 'success' : 
                                  capacityLevel.toLowerCase() === 'medium' ? 'warning' : 'danger';
            const rankEmoji = az.rank === 1 ? '🥇' : az.rank === 2 ? '🥈' : az.rank === 3 ? '🥉' : '';
            const intRate = az.interruptionRate ? az.interruptionRate.toFixed(1) + '%' : '-';
            
            return `
            <tr>
                <td>${rankEmoji} #${az.rank || i + 1}</td>
                <td><strong>${az.availabilityZone}</strong></td>
                <td>
                    <div style="display: flex; align-items: center; gap: 6px;">
                        <div style="width: 50px; height: 6px; background: var(--border-color); border-radius: 3px; overflow: hidden;">
                            <div style="width: ${score}%; height: 100%; background: linear-gradient(90deg, var(--primary-color), var(--secondary-color));"></div>
                        </div>
                        <span style="font-weight: 600;">${score.toFixed(0)}</span>
                    </div>
                </td>
                <td>
                    <span class="badge ${capacityClass}" style="margin-right: 4px;">${capacityLevel}</span>
                    <small style="color: var(--text-secondary);">${capacityScore.toFixed(0)}</small>
                </td>
                <td>$${az.avgPrice.toFixed(3)}/hr</td>
                <td>${intRate}</td>
                <td>${az.stability}</td>
            </tr>
        `;
        }).join('');
        
    } catch (error) {
        loading.innerHTML = `<p>❌ Error: ${error.message}</p>`;
    }
}

function closeAZModal() {
    document.getElementById('azModal').classList.add('hidden');
}

// AZ Lookup (standalone with its own cloud provider)
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
        // Use azCloudProvider (AZ Lookup's own cloud selection)
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                cloudProvider: state.azCloudProvider,
                instanceType,
                region
            })
        });
        
        const data = await response.json();
        
        loading.classList.add('hidden');
        results.classList.remove('hidden');
        
        const cloudLabels = { aws: 'AWS', azure: 'Azure', gcp: 'GCP' };
        const cloudLabel = cloudLabels[state.azCloudProvider] || 'AWS';
        const confidencePercent = (data.confidence * 100).toFixed(0);
        const confidenceClass = data.confidence >= 0.7 ? 'high' : data.confidence >= 0.4 ? 'medium' : 'low';

        // Render smart AZ results with capacity and interruption data
        results.innerHTML = `
            <div class="card">
                <div class="card-header">
                    <h3>${instanceType} in ${region} (${cloudLabel})</h3>
                    <span class="confidence-badge confidence-${confidenceClass}">
                        ${confidencePercent}% Confidence
                    </span>
                </div>
                <div class="card-body">
                    ${data.bestAz ? `<p><strong>🏆 Best AZ:</strong> ${data.bestAz}</p>` : ''}
                    ${data.nextBestAz ? `<p><strong>🔄 Backup AZ:</strong> ${data.nextBestAz}</p>` : ''}
                    
                    <!-- Insights -->
                    ${data.insights && data.insights.length > 0 ? `
                        <div class="insights-section">
                            <h4>💡 Insights</h4>
                            <ul class="insights-list">
                                ${data.insights.map(i => `<li>${i}</li>`).join('')}
                            </ul>
                        </div>
                    ` : ''}
                    
                    <!-- Smart AZ Rankings Table -->
                    <table class="data-table smart-az-table">
                        <thead>
                            <tr>
                                <th>Rank</th>
                                <th>AZ</th>
                                <th>Score</th>
                                <th>Capacity</th>
                                <th>Price</th>
                                <th>Int. Rate</th>
                                <th>Stability</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${(data.recommendations || []).map(az => {
                                const rankEmoji = az.rank === 1 ? '🥇' : az.rank === 2 ? '🥈' : az.rank === 3 ? '🥉' : '';
                                const capacityClass = az.capacityLevel === 'High' ? 'capacity-high' : 
                                                      az.capacityLevel === 'Medium' ? 'capacity-medium' : 'capacity-low';
                                const priceDisplay = az.pricePredicted ? `~$${az.avgPrice.toFixed(4)}` : `$${az.avgPrice.toFixed(4)}`;
                                const priceClass = az.pricePredicted ? 'price-predicted' : '';
                                
                                return `
                                    <tr class="${!az.available ? 'az-unavailable' : ''}">
                                        <td>${rankEmoji} #${az.rank}</td>
                                        <td><strong>${az.availabilityZone}</strong></td>
                                        <td>
                                            <div class="score-bar" style="--score: ${az.combinedScore}%">
                                                <span class="score-value">${az.combinedScore.toFixed(1)}</span>
                                            </div>
                                        </td>
                                        <td>
                                            <span class="capacity-badge ${capacityClass}">${az.capacityLevel}</span>
                                            <small>(${az.capacityScore.toFixed(0)})</small>
                                        </td>
                                        <td class="${priceClass}">${priceDisplay}</td>
                                        <td>${az.interruptionRate ? az.interruptionRate.toFixed(1) + '%' : 'N/A'}</td>
                                        <td>${az.stability}</td>
                                    </tr>
                                `;
                            }).join('')}
                        </tbody>
                    </table>
                    
                    <!-- Data Sources -->
                    ${data.dataSources && data.dataSources.length > 0 ? `
                        <div class="data-sources">
                            <small>📊 Data sources: ${data.dataSources.join(', ')}</small>
                        </div>
                    ` : ''}
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

// ========================================
// Instance Type Autocomplete
// ========================================

let autocompleteCache = {};
let selectedAutocompleteIndex = -1;

// Debounce helper
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

// Handle input in instance type field
async function handleInstanceTypeInput() {
    const input = document.getElementById('azInstanceType');
    const dropdown = document.getElementById('azInstanceSuggestions');
    const rawQuery = input.value.trim();

    if (rawQuery.length < 1) {
        hideAutocompleteDropdown();
        return;
    }

    // Parse query for various filters
    const queryParts = rawQuery.toLowerCase().split(/\s+/);
    let instanceQuery = '';
    let archFilter = '';
    let sizeFilter = '';
    let filters = [];

    // Size keywords
    const sizes = ['nano', 'micro', 'small', 'medium', 'large', 'xlarge', '2xlarge', '4xlarge', '8xlarge', '12xlarge', '16xlarge', '24xlarge', '32xlarge', '48xlarge', 'metal'];

    queryParts.forEach(part => {
        // Architecture filters
        if (part === 'amd' || part === 'ryzen' || part === 'intel' || part === 'x86' || part === 'x86_64') {
            archFilter = 'x86_64';
            filters.push('x86_64');
        } else if (part === 'arm' || part === 'arm64' || part === 'graviton') {
            archFilter = 'arm64';
            filters.push('ARM/Graviton');
        }
        // Size filters
        else if (sizes.includes(part)) {
            sizeFilter = part;
            filters.push(part);
        }
        // Everything else is part of instance type query
        else {
            instanceQuery += (instanceQuery ? '' : '') + part;
        }
    });

    // Build cache key
    const cacheKey = `${instanceQuery}|${archFilter}|${sizeFilter}`;

    // Check cache first
    if (autocompleteCache[cacheKey]) {
        renderAutocompleteResults(autocompleteCache[cacheKey], filters);
        return;
    }

    // Show loading state
    dropdown.innerHTML = '<div class="autocomplete-loading">🔍 Searching...</div>';
    dropdown.classList.remove('hidden');

    try {
        // Include cloud provider in the API call
        const cloud = state.cloudProvider || 'aws';
        const response = await fetch(`/api/instance-types?cloud=${cloud}&q=${encodeURIComponent(instanceQuery)}&limit=100`);
        const data = await response.json();

        if (data.success && data.instances) {
            let results = data.instances;

            // Filter by architecture if specified
            if (archFilter) {
                results = results.filter(inst => inst.architecture === archFilter);
            }

            // Filter by size if specified
            if (sizeFilter) {
                results = results.filter(inst => {
                    const instLower = inst.instanceType.toLowerCase();
                    // Handle size matching (e.g., "large" should match "large" but not "xlarge" unless specified)
                    if (sizeFilter === 'large') {
                        return instLower.includes('.large') || instLower.endsWith('large');
                    } else if (sizeFilter === 'xlarge') {
                        return instLower.includes('.xlarge') && !instLower.includes('2xlarge');
                    } else {
                        return instLower.includes(sizeFilter);
                    }
                });
            }

            autocompleteCache[cacheKey] = results;
            renderAutocompleteResults(results, filters);
        } else {
            dropdown.innerHTML = '<div class="autocomplete-empty">No instances found</div>';
        }
    } catch (error) {
        dropdown.innerHTML = '<div class="autocomplete-empty">Error fetching instances</div>';
    }
}

// Render autocomplete results
function renderAutocompleteResults(instances, filters = []) {
    const dropdown = document.getElementById('azInstanceSuggestions');
    selectedAutocompleteIndex = -1;

    if (!instances || instances.length === 0) {
        const filterMsg = filters.length > 0 ? ` with filters: ${filters.join(', ')}` : '';
        dropdown.innerHTML = `<div class="autocomplete-empty">No matching instances found${filterMsg}</div>`;
        dropdown.classList.remove('hidden');
        return;
    }

    // Show filter indicator if active
    const filterHeader = filters.length > 0 ?
        `<div class="autocomplete-filter-header">🔍 Filters: ${filters.join(' + ')} (${instances.length} results)</div>` : '';

    dropdown.innerHTML = filterHeader + instances.slice(0, 25).map((inst, index) => `
        <div class="autocomplete-item" data-index="${index}" data-value="${inst.instanceType}" onclick="selectAutocompleteItem('${inst.instanceType}')">
            <span class="autocomplete-item-name">${inst.instanceType}</span>
            <div class="autocomplete-item-details">
                <span>${inst.vcpu} vCPU</span>
                <span>${inst.memoryGb} GB</span>
                <span class="autocomplete-item-tag">${inst.architecture}</span>
            </div>
        </div>
    `).join('');

    dropdown.classList.remove('hidden');
}

// Handle keyboard navigation in autocomplete
function handleAutocompleteKeydown(e) {
    const dropdown = document.getElementById('azInstanceSuggestions');
    const items = dropdown.querySelectorAll('.autocomplete-item');

    if (dropdown.classList.contains('hidden') || items.length === 0) return;

    switch(e.key) {
        case 'ArrowDown':
            e.preventDefault();
            selectedAutocompleteIndex = Math.min(selectedAutocompleteIndex + 1, items.length - 1);
            updateAutocompleteSelection(items);
            break;
        case 'ArrowUp':
            e.preventDefault();
            selectedAutocompleteIndex = Math.max(selectedAutocompleteIndex - 1, 0);
            updateAutocompleteSelection(items);
            break;
        case 'Enter':
            e.preventDefault();
            if (selectedAutocompleteIndex >= 0 && items[selectedAutocompleteIndex]) {
                const value = items[selectedAutocompleteIndex].dataset.value;
                selectAutocompleteItem(value);
            }
            break;
        case 'Escape':
            hideAutocompleteDropdown();
            break;
    }
}

// Update visual selection in autocomplete
function updateAutocompleteSelection(items) {
    items.forEach((item, index) => {
        item.classList.toggle('active', index === selectedAutocompleteIndex);
    });

    // Scroll into view
    if (items[selectedAutocompleteIndex]) {
        items[selectedAutocompleteIndex].scrollIntoView({ block: 'nearest' });
    }
}

// Select an autocomplete item
function selectAutocompleteItem(value) {
    const input = document.getElementById('azInstanceType');
    input.value = value;
    hideAutocompleteDropdown();
    input.focus();
}

// Hide autocomplete dropdown
function hideAutocompleteDropdown() {
    const dropdown = document.getElementById('azInstanceSuggestions');
    if (dropdown) {
        dropdown.classList.add('hidden');
        selectedAutocompleteIndex = -1;
    }
}
