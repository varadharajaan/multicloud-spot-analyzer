// Spot Analyzer - Web UI Application
document.addEventListener('DOMContentLoaded', () => {
    initPresets();
    initCloudButtons();
    initArchButtons();
    initEventListeners();
    initCacheStatus();
    initFamilies();
});

// State
let selectedCloud = 'aws';
let selectedArch = 'any';
let selectedPreset = null;
let selectedFamilies = [];

// Cache status management
async function initCacheStatus() {
    await updateCacheStatus();
    
    // Add refresh button handler
    document.getElementById('cacheRefreshBtn').addEventListener('click', refreshCache);
    
    // Update cache status every 30 seconds
    setInterval(updateCacheStatus, 30000);
}

async function updateCacheStatus() {
    try {
        const response = await fetch('/api/cache/status');
        const data = await response.json();
        
        const cacheInfo = document.getElementById('cacheInfo');
        
        if (data.items > 0) {
            const hitRate = data.hits + data.misses > 0 
                ? Math.round((data.hits / (data.hits + data.misses)) * 100) 
                : 0;
            cacheInfo.textContent = `Cached: ${data.items} items | Hit rate: ${hitRate}% | TTL: ${data.ttlHours}h`;
        } else {
            cacheInfo.textContent = 'Cache empty - fresh data will be fetched';
        }
    } catch (error) {
        document.getElementById('cacheInfo').textContent = 'Cache status unavailable';
    }
}

async function refreshCache() {
    const btn = document.getElementById('cacheRefreshBtn');
    const originalText = btn.innerHTML;
    
    btn.innerHTML = '⏳ Refreshing...';
    btn.classList.add('cache-refreshing');
    
    try {
        const response = await fetch('/api/cache/refresh', { method: 'POST' });
        const data = await response.json();
        
        if (data.success) {
            btn.innerHTML = '✅ Refreshed!';
            await updateCacheStatus();
            
            setTimeout(() => {
                btn.innerHTML = originalText;
                btn.classList.remove('cache-refreshing');
            }, 2000);
        } else {
            throw new Error(data.error);
        }
    } catch (error) {
        btn.innerHTML = '❌ Failed';
        setTimeout(() => {
            btn.innerHTML = originalText;
            btn.classList.remove('cache-refreshing');
        }, 2000);
    }
}

// Update data freshness indicator
function updateDataFreshness(data) {
    // Data source
    const dataSourceValue = document.getElementById('dataSourceValue');
    if (data.dataSource) {
        dataSourceValue.textContent = data.dataSource;
        if (data.dataSource.includes('DescribeSpotPriceHistory')) {
            dataSourceValue.classList.add('live');
            dataSourceValue.classList.remove('cached');
        } else {
            dataSourceValue.classList.remove('live');
        }
    } else {
        dataSourceValue.textContent = 'AWS Spot Advisor';
    }

    // Cache status
    const cacheStatusValue = document.getElementById('cacheStatusValue');
    const cacheIcon = document.getElementById('cacheIcon');
    if (data.cachedData) {
        cacheStatusValue.textContent = 'Cached (2h TTL)';
        cacheStatusValue.classList.add('cached');
        cacheStatusValue.classList.remove('live');
        cacheIcon.textContent = '💾';
    } else {
        cacheStatusValue.textContent = 'Fresh fetch';
        cacheStatusValue.classList.add('live');
        cacheStatusValue.classList.remove('cached');
        cacheIcon.textContent = '🔄';
    }

    // Analyzed at timestamp
    const analyzedAtValue = document.getElementById('analyzedAtValue');
    if (data.analyzedAt) {
        const date = new Date(data.analyzedAt);
        const timeAgo = getTimeAgo(date);
        analyzedAtValue.textContent = `${date.toLocaleTimeString()} (${timeAgo})`;
    } else {
        analyzedAtValue.textContent = 'Just now';
    }
}

// Helper function to calculate time ago
function getTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    if (seconds < 60) return 'just now';
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
}

// Initialize presets
async function initPresets() {
    try {
        const response = await fetch('/api/presets');
        const presets = await response.json();
        renderPresets(presets);
    } catch (error) {
        console.error('Failed to load presets:', error);
        // Fallback presets
        const fallbackPresets = [
            { id: 'kubernetes', name: 'Kubernetes', description: 'Stable K8s nodes', icon: '☸️', minVcpu: 2, minMemory: 4, interruption: 1 },
            { id: 'database', name: 'Database', description: 'Max stability', icon: '🗄️', minVcpu: 2, minMemory: 8, interruption: 0 },
            { id: 'asg', name: 'Auto Scaling', description: 'Balanced ASG', icon: '📈', minVcpu: 2, minMemory: 4, interruption: 2 },
            { id: 'batch', name: 'Batch Jobs', description: 'Cost savings', icon: '⏰', minVcpu: 2, minMemory: 4, interruption: 3 },
            { id: 'web', name: 'Web Server', description: 'General purpose', icon: '🌐', minVcpu: 2, minMemory: 4, interruption: 2 },
            { id: 'ml', name: 'ML Training', description: 'Compute-optimized', icon: '🤖', minVcpu: 8, minMemory: 32, interruption: 2 }
        ];
        renderPresets(fallbackPresets);
    }
}

function renderPresets(presets) {
    const grid = document.getElementById('presetsGrid');
    grid.innerHTML = presets.map(preset => `
        <div class="preset-card" data-preset='${JSON.stringify(preset)}'>
            <div class="preset-icon">${preset.icon}</div>
            <div class="preset-name">${preset.name}</div>
            <div class="preset-desc">${preset.description}</div>
        </div>
    `).join('');

    // Add click handlers
    grid.querySelectorAll('.preset-card').forEach(card => {
        card.addEventListener('click', () => selectPreset(card));
    });
}

function selectPreset(card) {
    // Remove active from all
    document.querySelectorAll('.preset-card').forEach(c => c.classList.remove('active'));
    card.classList.add('active');

    const preset = JSON.parse(card.dataset.preset);
    selectedPreset = preset.id;

    // Apply preset values
    document.getElementById('minVcpu').value = preset.minVcpu || 2;
    document.getElementById('minMemory').value = preset.minMemory || 4;
    document.getElementById('interruption').value = preset.interruption ?? 2;
}

// Architecture buttons
function initArchButtons() {
    document.querySelectorAll('.arch-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.arch-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            selectedArch = btn.dataset.arch;
        });
    });
}

// Cloud provider buttons
function initCloudButtons() {
    document.querySelectorAll('.cloud-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.cloud-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            selectedCloud = btn.dataset.cloud;
            updateRegionsForCloud(selectedCloud);
            initFamilies(); // Reload families for selected cloud
        });
    });
}

// Update region dropdown for selected cloud
function updateRegionsForCloud(cloud) {
    const regionSelect = document.getElementById('region');
    const awsOptgroups = document.querySelectorAll('[id^="aws-"]');
    const azureOptgroups = document.querySelectorAll('[id^="azure-"]');

    if (cloud === 'azure') {
        awsOptgroups.forEach(og => og.classList.add('hidden'));
        azureOptgroups.forEach(og => og.classList.remove('hidden'));
        regionSelect.value = 'eastus';
    } else {
        azureOptgroups.forEach(og => og.classList.add('hidden'));
        awsOptgroups.forEach(og => og.classList.remove('hidden'));
        regionSelect.value = 'us-east-1';
    }
}

// Instance Families
async function initFamilies() {
    try {
        const response = await fetch(`/api/families?cloud=${selectedCloud}`);
        const families = await response.json();

        const container = document.getElementById('familyChips');
        if (!container) return;

        // Reset selected families when switching clouds
        selectedFamilies = [];

        container.innerHTML = families.map(f => `
            <button class="family-chip" data-family="${f.Name || f.name}">
                <span class="family-chip-name">${f.Name || f.name}</span>
                <span class="family-chip-desc">${f.Description || f.description || ''}</span>
            </button>
        `).join('');

        // Update badge
        const badge = document.getElementById('familyCount');
        if (badge) badge.textContent = 'All';

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
    selectedFamilies = Array.from(chips).map(c => c.dataset.family);

    const badge = document.getElementById('familyCount');
    if (badge) {
        badge.textContent = selectedFamilies.length > 0
            ? selectedFamilies.length
            : 'All';
    }
}

// Event listeners
function initEventListeners() {
    // Parse requirements button
    document.getElementById('parseBtn').addEventListener('click', parseRequirements);

    // Analyze button
    document.getElementById('analyzeBtn').addEventListener('click', analyze);

    // Enter key in textarea
    document.getElementById('nlInput').addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && e.ctrlKey) {
            parseRequirements();
        }
    });
}

// Parse natural language requirements
async function parseRequirements() {
    const text = document.getElementById('nlInput').value.trim();
    if (!text) return;

    try {
        const response = await fetch('/api/parse-requirements', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ text })
        });

        const result = await response.json();
        
        // Apply parsed values
        if (result.minVcpu) document.getElementById('minVcpu').value = result.minVcpu;
        if (result.maxVcpu) document.getElementById('maxVcpu').value = result.maxVcpu;
        if (result.minMemory) document.getElementById('minMemory').value = result.minMemory;
        if (result.maxMemory) document.getElementById('maxMemory').value = result.maxMemory;
        if (result.maxInterruption !== undefined) document.getElementById('interruption').value = result.maxInterruption;
        
        if (result.architecture) {
            document.querySelectorAll('.arch-btn').forEach(btn => {
                btn.classList.remove('active');
                if (btn.dataset.arch === result.architecture) {
                    btn.classList.add('active');
                    selectedArch = result.architecture;
                }
            });
        }

        if (result.useCase) {
            selectedPreset = result.useCase;
            // Highlight matching preset
            document.querySelectorAll('.preset-card').forEach(card => {
                const preset = JSON.parse(card.dataset.preset);
                card.classList.toggle('active', preset.id === result.useCase);
            });
        }

        // Show explanation
        const parseResult = document.getElementById('parseResult');
        parseResult.classList.remove('hidden');
        parseResult.innerHTML = `
            <strong>✨ Parsed:</strong> ${result.explanation}
            <br><small style="color: #718096;">Click "Find Best Instances" to analyze</small>
        `;

    } catch (error) {
        console.error('Parse failed:', error);
    }
}

// Analyze instances
async function analyze() {
    const loading = document.getElementById('loading');
    const results = document.getElementById('results');
    
    loading.classList.remove('hidden');
    results.classList.add('hidden');

    const request = {
        cloudProvider: selectedCloud,
        minVcpu: parseInt(document.getElementById('minVcpu').value) || 2,
        maxVcpu: parseInt(document.getElementById('maxVcpu').value) || 0,
        minMemory: parseInt(document.getElementById('minMemory').value) || 4,
        maxMemory: parseInt(document.getElementById('maxMemory').value) || 0,
        architecture: selectedArch === 'any' ? '' : selectedArch,
        region: document.getElementById('region').value,
        maxInterruption: parseInt(document.getElementById('interruption').value),
        useCase: selectedPreset || 'general',
        enhanced: document.getElementById('enhanced').checked,
        topN: parseInt(document.getElementById('topN').value) || 10,
        families: selectedFamilies.length > 0 ? selectedFamilies : []
    };

    try {
        const response = await fetch('/api/analyze', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(request)
        });

        const data = await response.json();
        
        loading.classList.add('hidden');

        if (data.success) {
            renderResults(data);
        } else {
            alert('Analysis failed: ' + data.error);
        }

    } catch (error) {
        loading.classList.add('hidden');
        alert('Request failed: ' + error.message);
    }
}

// Render results
function renderResults(data) {
    const results = document.getElementById('results');
    const region = document.getElementById('region').value;
    results.classList.remove('hidden');

    // Summary
    document.getElementById('summary').innerHTML = `🎯 ${data.summary}`;

    // Insights
    const insightsHtml = data.insights.map(i => 
        `<span class="insight-badge">${i}</span>`
    ).join('');
    document.getElementById('insights').innerHTML = insightsHtml;

    // Data freshness indicator
    updateDataFreshness(data);

    // Table with auto-fetched Best AZ
    const tbody = document.getElementById('resultsBody');
    tbody.innerHTML = data.instances.map(inst => `
        <tr>
            <td>
                <span class="rank-badge rank-${inst.rank <= 3 ? inst.rank : 'default'}">
                    ${inst.rank}
                </span>
            </td>
            <td><strong>${inst.instanceType}</strong></td>
            <td>${inst.vcpu}</td>
            <td>${inst.memoryGb} GB</td>
            <td><span class="savings-badge">${inst.savingsPercent}%</span></td>
            <td>
                <span class="interruption-badge ${getInterruptionClass(inst.interruptionLevel)}">
                    ${inst.interruptionLevel}
                </span>
            </td>
            <td>
                <div class="score-bar">
                    <div class="score-fill" style="width: ${inst.score * 100}%"></div>
                </div>
                <small>${(inst.score * 100).toFixed(1)}%</small>
            </td>
            <td><span class="arch-badge">${inst.architecture}</span></td>
            <td>
                <span class="az-cell" data-instance="${inst.instanceType}" data-region="${region}" 
                      onclick="showAZRecommendation('${inst.instanceType}', '${region}')" 
                      title="Click for top 3 AZs" style="cursor: pointer;">
                    <span class="az-value">⏳</span>
                </span>
            </td>
        </tr>
    `).join('');
    
    // Auto-fetch Best AZ for all rows
    autoFetchAllAZs(tbody, region);

    // Scroll to results
    results.scrollIntoView({ behavior: 'smooth' });
}

// AZ Recommendation Modal
async function showAZRecommendation(instanceType, region) {
    const modal = document.getElementById('azModal');
    const loading = document.getElementById('azLoading');
    const results = document.getElementById('azResults');
    
    document.getElementById('azInstanceType').textContent = instanceType;
    modal.classList.remove('hidden');
    loading.classList.remove('hidden');
    results.classList.add('hidden');
    
    // Clear previous results to prevent showing stale data
    document.getElementById('azInsights').innerHTML = '';
    document.getElementById('azResultsBody').innerHTML = '';
    document.getElementById('azPriceDiff').innerHTML = '';

    try {
        const response = await fetch('/api/az', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ cloudProvider: selectedCloud, instanceType, region })
        });

        const data = await response.json();
        loading.classList.add('hidden');

        if (data.success) {
            renderAZResults(data);
        } else {
            document.getElementById('azInsights').innerHTML = `
                <div class="az-insight" style="color: #e53e3e;">⚠️ ${data.error}</div>
            `;
            results.classList.remove('hidden');
        }
    } catch (error) {
        loading.classList.add('hidden');
        document.getElementById('azInsights').innerHTML = `
            <div class="az-insight" style="color: #e53e3e;">⚠️ Failed to fetch AZ data: ${error.message}</div>
        `;
        results.classList.remove('hidden');
    }
}

function renderAZResults(data) {
    const results = document.getElementById('azResults');
    results.classList.remove('hidden');

    // Insights
    const insightsHtml = data.insights.map(i => 
        `<div class="az-insight">${i}</div>`
    ).join('');
    document.getElementById('azInsights').innerHTML = insightsHtml;

    // Table
    const tbody = document.getElementById('azResultsBody');
    if (data.recommendations && data.recommendations.length > 0) {
        tbody.innerHTML = data.recommendations.map(az => {
            const rankEmoji = az.rank === 1 ? '🥇' : az.rank === 2 ? '🥈' : az.rank === 3 ? '🥉' : '';
            const stabilityClass = az.stability.toLowerCase().replace(' ', '-');
            return `
                <tr>
                    <td>${rankEmoji} #${az.rank}</td>
                    <td><strong>${az.availabilityZone}</strong></td>
                    <td>$${az.avgPrice.toFixed(3)}/hr</td>
                    <td>$${az.currentPrice.toFixed(3)}/hr</td>
                    <td>$${az.minPrice.toFixed(3)}/hr</td>
                    <td>$${az.maxPrice.toFixed(3)}/hr</td>
                    <td>
                        <span class="stability-badge stability-${stabilityClass}">${az.stability}</span>
                    </td>
                </tr>
            `;
        }).join('');

        // Price differential
        if (data.priceDifferential > 0) {
            document.getElementById('azPriceDiff').innerHTML = `
                💰 <strong>${data.priceDifferential.toFixed(1)}%</strong> price difference between best and worst AZ
                <br><small>Best AZ: <strong>${data.bestAz}</strong></small>
            `;
        }
    } else {
        tbody.innerHTML = `<tr><td colspan="7" style="text-align: center; padding: 20px;">No AZ data available. Configure AWS credentials for real-time pricing.</td></tr>`;
        document.getElementById('azPriceDiff').innerHTML = '';
    }
}

function closeAZModal() {
    document.getElementById('azModal').classList.add('hidden');
}

// Close modal on escape key
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeAZModal();
});

// Auto-fetch Best AZ for all instances
async function autoFetchAllAZs(tbody, region) {
    const azCells = tbody.querySelectorAll('.az-cell');
    
    // Fetch all AZs in parallel (batch of 5 to avoid overwhelming)
    const batchSize = 5;
    for (let i = 0; i < azCells.length; i += batchSize) {
        const batch = Array.from(azCells).slice(i, i + batchSize);
        await Promise.all(batch.map(cell => fetchAZForCell(cell)));
    }
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
            valueSpan.innerHTML = `<strong style="color: var(--accent-color, #667eea);">${data.bestAz}</strong>`;
        } else {
            valueSpan.textContent = 'N/A';
        }
    } catch (e) {
        valueSpan.textContent = '❌';
    }
}

// Close modal on outside click
document.getElementById('azModal')?.addEventListener('click', (e) => {
    if (e.target.id === 'azModal') closeAZModal();
});

function getInterruptionClass(level) {
    if (level.includes('<5') || level.includes('5-10')) return 'int-low';
    if (level.includes('10-15') || level.includes('15-20')) return 'int-med';
    return 'int-high';
}
