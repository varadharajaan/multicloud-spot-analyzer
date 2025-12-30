// Spot Analyzer - Web UI Application
document.addEventListener('DOMContentLoaded', () => {
    initPresets();
    initArchButtons();
    initEventListeners();
});

// State
let selectedArch = 'any';
let selectedPreset = null;

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
        minVcpu: parseInt(document.getElementById('minVcpu').value) || 2,
        maxVcpu: parseInt(document.getElementById('maxVcpu').value) || 0,
        minMemory: parseInt(document.getElementById('minMemory').value) || 4,
        maxMemory: parseInt(document.getElementById('maxMemory').value) || 0,
        architecture: selectedArch === 'any' ? '' : selectedArch,
        region: document.getElementById('region').value,
        maxInterruption: parseInt(document.getElementById('interruption').value),
        useCase: selectedPreset || 'general',
        enhanced: document.getElementById('enhanced').checked,
        topN: 10
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
    results.classList.remove('hidden');

    // Summary
    document.getElementById('summary').innerHTML = `🎯 ${data.summary}`;

    // Insights
    const insightsHtml = data.insights.map(i => 
        `<span class="insight-badge">${i}</span>`
    ).join('');
    document.getElementById('insights').innerHTML = insightsHtml;

    // Table
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
        </tr>
    `).join('');

    // Scroll to results
    results.scrollIntoView({ behavior: 'smooth' });
}

function getInterruptionClass(level) {
    if (level.includes('<5') || level.includes('5-10')) return 'int-low';
    if (level.includes('10-15') || level.includes('15-20')) return 'int-med';
    return 'int-high';
}
