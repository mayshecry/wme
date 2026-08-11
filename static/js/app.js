const API = {
host: '/api/host',
hostStats: '/api/host/stats',
vms: '/api/vms',
storage: '/api/storage',
networks: '/api/networks'
};

const state = {
currentPage: 'dashboard',
currentVM: null,
hostPerfData: [],
vmPerfData: [],
ws: null,
wsRetries: 0,
vms: [],
vmStatsMap: {}
};

let hostPerfChart = null;
let vmPerfChart = null;

function init() {
setupNavigation();
setupWizard();
setupConfirmModal();
setupClock();
connectWebSocket();
loadVMs();
loadStorage();
loadNetworks();
loadHostInfo();
initCharts();
bindEvents();
}

function setupNavigation() {
document.querySelectorAll('.nav-item').forEach(item => {
item.addEventListener('click', (e) => {
e.preventDefault();
const page = item.dataset.page;
navigateTo(page);
});
});
}

function navigateTo(page) {
state.currentPage = page;
document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
document.querySelector(`.nav-item[data-page="${page}"]`).classList.add('active');
document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
document.getElementById(`page-${page}`).classList.add('active');

const titles = {
dashboard: 'Dashboard',
vms: 'Virtual Machines',
storage: 'Storage',
networks: 'Networks',
'vm-detail': `VM: ${state.currentVM || ''}`
};
document.getElementById('pageTitle').textContent = titles[page] || 'Dashboard';

if (page === 'vms') {
loadVMs();
} else if (page === 'storage') {
loadStorage();
} else if (page === 'networks') {
loadNetworks();
} else if (page === 'vm-detail' && state.currentVM) {
loadVMDetail(state.currentVM);
}
}

function bindEvents() {
document.getElementById('createVmBtn').addEventListener('click', () => openWizard());
document.getElementById('refreshVmsBtn').addEventListener('click', () => loadVMs());
document.getElementById('backToVmsBtn').addEventListener('click', () => navigateTo('vms'));
}

function setupClock() {
const updateClock = () => {
const now = new Date();
document.getElementById('clock').textContent = now.toLocaleString();
};
updateClock();
setInterval(updateClock, 1000);
}

function connectWebSocket() {
const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const url = `${protocol}//${window.location.host}/api/ws`;

try {
state.ws = new WebSocket(url);

state.ws.onopen = () => {
state.wsRetries = 0;
setHostStatus(true);
};

state.ws.onmessage = (event) => {
try {
const data = JSON.parse(event.data);
if (data.type === 'stats') {
updateHostStats(data.host_stats);
updateVMStats(data.vm_stats);
state.vms.forEach(vm => {
const stats = data.vm_stats.find(s => s.name === vm.name);
if (stats) {
vm.stats = stats;
}
});
if (state.currentPage === 'vms') {
updateVMsTable(state.vms);
}
updateDashboardVMs(state.vms);
}
} catch (e) {
console.error('WS parse error:', e);
}
};

state.ws.onclose = () => {
setHostStatus(false);
scheduleReconnect();
};

state.ws.onerror = (error) => {
console.error('WebSocket error:', error);
setHostStatus(false);
};
} catch (e) {
console.error('WS connection error:', e);
scheduleReconnect();
}
}

function scheduleReconnect() {
if (state.wsRetries >= 5) {
setHostStatus(false);
return;
}
state.wsRetries++;
setTimeout(connectWebSocket, 3000 * state.wsRetries);
}

function setHostStatus(online) {
const dot = document.getElementById('hostStatusDot');
const text = document.getElementById('hostStatusText');
if (online) {
dot.classList.remove('offline');
text.textContent = 'Connected';
} else {
dot.classList.add('offline');
text.textContent = 'Disconnected';
}
}

async function apiFetch(url, options = {}) {
const res = await fetch(url, options);
if (!res.ok) {
const err = await res.json().catch(() => ({}));
throw new Error(err.error || `HTTP ${res.status}`);
}
return res.json();
}

function showToast(message, type = 'info') {
const container = document.getElementById('toastContainer');
const toast = document.createElement('div');
toast.className = `toast ${type}`;
const icons = {
success: 'fa-check-circle',
error: 'fa-exclamation-circle',
info: 'fa-info-circle',
warning: 'fa-exclamation-triangle'
};
toast.innerHTML = `<i class="fas ${icons[type] || icons.info}"></i> ${message}`;
container.appendChild(toast);
setTimeout(() => {
toast.style.opacity = '0';
toast.style.transition = 'opacity 0.3s';
setTimeout(() => toast.remove(), 300);
}, 4000);
}

async function loadVMs() {
	const tbody = document.getElementById('vmsTableBody');
	tbody.innerHTML = '<tr><td colspan="7" class="text-center">Loading...</td></tr>';
	try {
		const vms = await apiFetch(API.vms);
		state.vms = vms;
		if (state.currentPage === 'vms') {
			updateVMsTable(vms);
		}
		updateDashboardVMs(vms);
		if (vms.length > 0) {
			setTimeout(() => {
				vms.forEach(vm => {
					if (vm.state === 'running') {
						fetchAndUpdateIP(vm.name);
					}
				});
			}, 1000);
		}
	} catch (err) {
		tbody.innerHTML = `<tr><td colspan="7" class="text-center">Error: ${err.message}</td></tr>`;
		showToast(`Failed to load VMs: ${err.message}`, 'error');
		setHostStatus(false);
	}
}

async function refreshVMs() {
try {
const vms = await apiFetch(API.vms);
state.vms = vms;
if (state.currentPage === 'vms') {
updateVMsTable(vms);
}
updateDashboardVMs(vms);
} catch (err) {
showToast(`Failed to refresh VMs: ${err.message}`, 'error');
}
}

function updateVMsTable(vms) {
const tbody = document.getElementById('vmsTableBody');
if (!vms || vms.length === 0) {
tbody.innerHTML = '<tr><td colspan="7" class="text-center">No virtual machines found</td></tr>';
return;
}

tbody.innerHTML = vms.map(vm => {
const stateClass = getStateClass(vm.state);
const cpu = vm.stats ? vm.stats.cpu_usage.toFixed(1) : '-';
const mem = vm.stats && vm.stats.memory_max ? `${formatBytes(vm.stats.memory_used)} / ${formatBytes(vm.stats.memory_max)}` : '-';
const ips = vm.ips && vm.ips.length > 0 ? vm.ips.map(ip => `<span class="ip-badge">${escapeHtml(ip)}</span>`).join('') : '<span class="text-muted">-</span>';
return `
<tr>
<td>${vm.id === 0 ? '-' : vm.id}</td>
<td><a href="#" class="vm-name-link" data-name="${escapeHtml(vm.name)}">${escapeHtml(vm.name)}</a></td>
<td><span class="state-badge ${stateClass}">${escapeHtml(vm.state)}</span></td>
<td>${ips}</td>
<td>${cpu}%</td>
<td>${mem}</td>
<td>
<div class="action-buttons">
${vm.state === 'running' ? `
<button class="action-btn shutdown" title="Shutdown" onclick="vmAction('${escapeAttr(vm.name)}', 'shutdown')"><i class="fas fa-power-off"></i></button>
<button class="action-btn reboot" title="Reboot" onclick="vmAction('${escapeAttr(vm.name)}', 'reboot')"><i class="fas fa-sync"></i></button>
<button class="action-btn suspend" title="Suspend" onclick="vmAction('${escapeAttr(vm.name)}', 'suspend')"><i class="fas fa-pause"></i></button>
` : ''}
${vm.state !== 'running' ? `
<button class="action-btn start" title="Start" onclick="vmAction('${escapeAttr(vm.name)}', 'start')"><i class="fas fa-play"></i></button>
` : ''}
${vm.state === 'paused' ? `
<button class="action-btn resume" title="Resume" onclick="vmAction('${escapeAttr(vm.name)}', 'resume')"><i class="fas fa-play"></i></button>
` : ''}
<button class="action-btn detail" title="Details" onclick="showVMDetail('${escapeAttr(vm.name)}')"><i class="fas fa-info-circle"></i></button>
<button class="action-btn stop" title="Delete" onclick="vmAction('${escapeAttr(vm.name)}', 'delete')"><i class="fas fa-trash"></i></button>
</div>
</td>
</tr>`;
	}).join('');

}

async function fetchAndUpdateIP(name) {
	try {
		const detail = await apiFetch(`${API.vms}/${encodeURIComponent(name)}`);
		if (detail.info && detail.info.ips && detail.info.ips.length > 0) {
			const row = document.querySelector(`tr:has(.vm-name-link[data-name="${name}"])`);
			if (row) {
				const ipCell = row.querySelector('td:nth-child(4)');
				if (ipCell && ipCell.querySelector('.text-muted')) {
					ipCell.innerHTML = detail.info.ips.map(ip => `<span class="ip-badge">${escapeHtml(ip)}</span>`).join('');
				}
			}
			const vm = state.vms.find(v => v.name === name);
			if (vm) {
				vm.ips = detail.info.ips;
				updateDashboardVMs(state.vms);
			}
		}
	} catch (e) {
		console.error('Failed to fetch IP for', name, e);
	}
}

function getStateClass(state) {
if (state === 'running') return 'state-running';
if (state === 'crashed') return 'state-crashed';
return 'state-shut-off';
}

async function vmAction(name, action) {
const confirmMessages = {
delete: `Are you sure you want to delete VM "${name}"? This will remove all associated storage.`,
stop: `Are you sure you want to force stop VM "${name}"?`,
shutdown: `Send shutdown signal to VM "${name}"?`,
reboot: `Reboot VM "${name}"?`,
start: `Start VM "${name}"?`,
suspend: `Suspend VM "${name}"?`,
resume: `Resume VM "${name}"?`
};

if (['delete', 'stop'].includes(action)) {
showConfirm(
action === 'delete' ? 'Delete VM' : 'Force Stop',
confirmMessages[action],
async () => {
try {
await apiFetch(`/api/vm/${encodeURIComponent(name)}/${action}`, { method: 'POST' });
showToast(`${action} request sent for ${name}`, 'success');
await refreshVMs();
if (state.currentPage === 'vm-detail' && state.currentVM === name) {
loadVMDetail(name);
}
} catch (err) {
showToast(`Failed: ${err.message}`, 'error');
}
}
);
} else {
try {
await apiFetch(`/api/vm/${encodeURIComponent(name)}/${action}`, { method: 'POST' });
showToast(`${action} request sent for ${name}`, 'success');
await refreshVMs();
if (state.currentPage === 'vm-detail' && state.currentVM === name) {
loadVMDetail(name);
}
} catch (err) {
showToast(`Failed: ${err.message}`, 'error');
}
}
}

function showPreview(name) {
showConfirm('Console', `Open console for ${name}? NoVNC support requires configuration.`, () => {
showToast('Console support requires NoVNC setup', 'warning');
});
}

async function loadStorage() {
const tbody = document.getElementById('storageTableBody');
tbody.innerHTML = '<tr><td colspan="5" class="text-center">Loading...</td></tr>';
try {
const pools = await apiFetch(API.storage);
if (!pools || pools.length === 0) {
tbody.innerHTML = '<tr><td colspan="5" class="text-center">No storage pools found</td></tr>';
updateStorageSummary([]);
return;
}

const poolDetails = await Promise.all(pools.map(async pool => {
try {
const info = await apiFetch(`${API.storage}/${encodeURIComponent(pool.name)}`);
return { ...pool, ...info };
} catch {
return pool;
}
}));

tbody.innerHTML = poolDetails.map(pool => {
const capacity = pool.capacity || 0;
const allocation = pool.allocation || 0;
const available = pool.available || 0;
const usage = capacity > 0 ? (allocation / capacity * 100).toFixed(1) : '0';
return `
<tr>
<td>${escapeHtml(pool.name)}</td>
<td><span class="state-badge ${pool.active ? 'state-running' : 'state-shut-off'}">${escapeHtml(pool.state || (pool.active ? 'active' : 'inactive'))}</span></td>
<td>${formatBytes(capacity)}</td>
<td>${formatBytes(allocation)} (${usage}%)</td>
<td>${formatBytes(available)}</td>
</tr>`;
}).join('');

updateStorageSummary(poolDetails);
} catch (err) {
tbody.innerHTML = `<tr><td colspan="5" class="text-center">Error: ${err.message}</td></tr>`;
showToast(`Failed to load storage: ${err.message}`, 'error');
}
}

function updateStorageSummary(pools) {
const listEl = document.getElementById('storageSummaryList');
const totalEl = document.getElementById('storageTotal');
if (!pools || pools.length === 0) {
listEl.innerHTML = '<div class="empty-state">No storage pools</div>';
totalEl.textContent = '0 B total';
return;
}

let totalCapacity = 0;
let totalAllocation = 0;
pools.forEach(pool => {
totalCapacity += pool.capacity || 0;
totalAllocation += pool.allocation || 0;
});

totalEl.textContent = `${formatBytes(totalAllocation)} / ${formatBytes(totalCapacity)} used`;

listEl.innerHTML = pools.map(pool => {
const capacity = pool.capacity || 0;
const allocation = pool.allocation || 0;
const usage = capacity > 0 ? (allocation / capacity * 100) : 0;
return `
<div class="storage-summary-item">
<div class="storage-summary-info">
<span class="storage-summary-name">${escapeHtml(pool.name)}</span>
<span class="storage-summary-size">${formatBytes(allocation)} / ${formatBytes(capacity)}</span>
</div>
<div class="progress-bar">
<div class="progress-fill disk-fill" style="width:${Math.min(usage, 100)}%"></div>
</div>
</div>`;
}).join('');
}

async function loadHostInfo() {
try {
const info = await apiFetch(API.host);
const grid = document.getElementById('hostInfoGrid');
grid.innerHTML = `
<div class="info-item">
<div class="label">CPU Model</div>
<div class="value">${escapeHtml(info.cpu_model || '-')}</div>
</div>
<div class="info-item">
<div class="label">CPU Cores</div>
<div class="value">${info.cpus || '-'}</div>
</div>
<div class="info-item">
<div class="label">CPU Frequency</div>
<div class="value">${info.cpu_frequency ? info.cpu_frequency.toFixed(2) + ' MHz' : '-'}</div>
</div>
<div class="info-item">
<div class="label">Total Memory</div>
<div class="value">${formatBytes((info.memory_kb || 0) * 1024)}</div>
</div>
`;
} catch (err) {
console.error('Failed to load host info:', err);
}
}

async function loadNetworks() {
const tbody = document.getElementById('networksTableBody');
tbody.innerHTML = '<tr><td colspan="4" class="text-center">Loading...</td></tr>';
try {
const networks = await apiFetch(API.networks);
if (!networks || networks.length === 0) {
tbody.innerHTML = '<tr><td colspan="4" class="text-center">No networks found</td></tr>';
return;
}

const networkDetails = await Promise.all(networks.map(async net => {
try {
const info = await apiFetch(`${API.networks}/${encodeURIComponent(net.name)}`);
return { ...net, ...info };
} catch {
return net;
}
}));

tbody.innerHTML = networkDetails.map(net => `
<tr>
<td>${escapeHtml(net.name)}</td>
<td><span class="state-badge ${net.active ? 'state-running' : 'state-shut-off'}">${escapeHtml(net.state || (net.active ? 'active' : 'inactive'))}</span></td>
<td>${escapeHtml(net.bridge || '-')}</td>
<td>${escapeHtml(net.forward || '-')}</td>
</tr>
`).join('');
} catch (err) {
tbody.innerHTML = `<tr><td colspan="4" class="text-center">Error: ${err.message}</td></tr>`;
showToast(`Failed to load networks: ${err.message}`, 'error');
}
}

async function loadVMDetail(name) {
state.currentVM = name;
state.currentPage = 'vm-detail';
document.getElementById('vmDetailName').textContent = name;

document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
document.querySelector(`.nav-item[data-page="vms"]`).classList.add('active');
document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
document.getElementById('page-vm-detail').classList.add('active');
document.getElementById('pageTitle').textContent = `VM: ${name}`;

try {
const detail = await apiFetch(`${API.vms}/${encodeURIComponent(name)}`);
renderVMDetail(detail);
updateVMActionButtons(detail.info.state);
} catch (err) {
showToast(`Failed to load VM details: ${err.message}`, 'error');
}
}

function renderVMDetail(detail) {
const info = detail.info;
const infoGrid = document.getElementById('vmInfoGrid');
infoGrid.innerHTML = `
<div class="info-item">
<div class="label">Status</div>
<div class="value"><span class="state-badge ${getStateClass(info.state)}">${escapeHtml(info.state)}</span></div>
</div>
<div class="info-item">
<div class="label">DOM ID</div>
<div class="value">${info.id === 0 ? '-' : info.id}</div>
</div>
<div class="info-item">
<div class="label">vCPUs</div>
<div class="value">${info.vcpus}</div>
</div>
<div class="info-item">
<div class="label">Memory</div>
<div class="value">${formatBytes(info.max_memory)} / Max ${formatBytes(info.max_memory)}</div>
</div>
<div class="info-item">
<div class="label">Persistent</div>
<div class="value">${escapeHtml(info.persistent || 'yes')}</div>
</div>
<div class="info-item">
<div class="label">Autostart</div>
<div class="value">${escapeHtml(info.autostart || 'disable')}</div>
</div>
<div class="info-item">
<div class="label">IP Addresses</div>
<div class="value">${info.ips && info.ips.length > 0 ? info.ips.map(ip => `<span class="ip-badge">${escapeHtml(ip)}</span>`).join('') : '<span class="text-muted">-</span>'}</div>
</div>
`;

const disksBody = document.getElementById('vmDisksBody');
if (!detail.disks || detail.disks.length === 0) {
disksBody.innerHTML = '<tr><td colspan="3" class="text-center">No disks</td></tr>';
} else {
disksBody.innerHTML = detail.disks.map(disk => `
<tr>
<td>${escapeHtml(disk.device)}</td>
<td>${escapeHtml(disk.target)}</td>
<td>${escapeHtml(disk.source || '-')}</td>
</tr>
`).join('');
}

const nicsBody = document.getElementById('vmNicsBody');
if (!detail.nics || detail.nics.length === 0) {
nicsBody.innerHTML = '<tr><td colspan="4" class="text-center">No network interfaces</td></tr>';
} else {
nicsBody.innerHTML = detail.nics.map(nic => `
<tr>
<td>${escapeHtml(nic.type)}</td>
<td>${escapeHtml(nic.mac)}</td>
<td>${escapeHtml(nic.source || nic.network || '-')}</td>
<td>${escapeHtml(nic.model || '-')}</td>
</tr>
`).join('');
}
}

function updateVMActionButtons(state) {
const container = document.getElementById('vmDetailActions');
const name = state.currentVM;
container.innerHTML = '';

if (state === 'running') {
container.innerHTML += `<button class="btn btn-secondary btn-sm" onclick="vmAction('${escapeAttr(name)}', 'shutdown')"><i class="fas fa-power-off"></i> Shutdown</button>`;
container.innerHTML += `<button class="btn btn-secondary btn-sm" onclick="vmAction('${escapeAttr(name)}', 'reboot')"><i class="fas fa-sync"></i> Reboot</button>`;
container.innerHTML += `<button class="btn btn-secondary btn-sm" onclick="vmAction('${escapeAttr(name)}', 'suspend')"><i class="fas fa-pause"></i> Suspend</button>`;
container.innerHTML += `<button class="btn btn-danger btn-sm" onclick="vmAction('${escapeAttr(name)}', 'stop')"><i class="fas fa-stop"></i> Force Stop</button>`;
} else if (state === 'paused') {
container.innerHTML += `<button class="btn btn-success btn-sm" onclick="vmAction('${escapeAttr(name)}', 'resume')"><i class="fas fa-play"></i> Resume</button>`;
} else {
container.innerHTML += `<button class="btn btn-success btn-sm" onclick="vmAction('${escapeAttr(name)}', 'start')"><i class="fas fa-play"></i> Start</button>`;
}

container.innerHTML += `<button class="btn btn-danger btn-sm" onclick="vmAction('${escapeAttr(name)}', 'delete')"><i class="fas fa-trash"></i> Delete</button>`;
}

function updateHostStats(stats) {
const cpu = stats.cpu_usage || 0;
const memPct = stats.memory_usage || 0;
const diskPct = stats.disk_usage || 0;

document.getElementById('hostCpu').textContent = `${cpu.toFixed(1)}%`;
document.getElementById('hostCpuBar').style.width = `${Math.min(cpu, 100)}%`;

document.getElementById('hostMem').textContent = `${formatBytes(stats.memory_used)} / ${formatBytes(stats.memory_total)}`;
document.getElementById('hostMemBar').style.width = `${Math.min(memPct, 100)}%`;

document.getElementById('hostDisk').textContent = `${formatBytes(stats.disk_used)} / ${formatBytes(stats.disk_total)}`;
document.getElementById('hostDiskBar').style.width = `${Math.min(diskPct, 100)}%`;

document.getElementById('hostNet').textContent = `↓ ${formatRate(stats.net_rx_rate)} ↑ ${formatRate(stats.net_tx_rate)}`;
document.getElementById('hostNetTotal').textContent = `Total: ${formatBytes(stats.net_rx)} / ${formatBytes(stats.net_tx)}`;

const now = new Date();
state.hostPerfData.push({ time: now, cpu, mem: memPct, net: stats.net_rx_rate });
if (state.hostPerfData.length > 30) {
state.hostPerfData.shift();
}
updateHostChart();
}

function updateVMStats(vmStatsList) {
state.vmStatsMap = {};
vmStatsList.forEach(stats => {
state.vmStatsMap[stats.name] = stats;
});
}

function updateDashboardVMs(vms) {
const container = document.getElementById('vmOverviewList');
const countEl = document.getElementById('vmCount');
if (!vms || vms.length === 0) {
container.innerHTML = '<div class="empty-state">No virtual machines found. Click "Create VM" to get started.</div>';
countEl.textContent = '0 VMs';
return;
}

const runningCount = vms.filter(vm => vm.state === 'running').length;
countEl.textContent = `${vms.length} VMs (${runningCount} running)`;

container.innerHTML = vms.map(vm => {
const stats = vm.stats || state.vmStatsMap[vm.name];
const cpuPct = stats ? Math.min((stats.cpu_usage || 0), 100) : 0;
const isRunning = vm.state === 'running';
const ips = vm.ips && vm.ips.length > 0 ? vm.ips.map(ip => `<span class="ip-badge">${escapeHtml(ip)}</span>`).join('') : '';
return `
<div class="vm-overview-item" onclick="showVMDetail('${escapeAttr(vm.name)}')">
<div class="vm-overview-info">
<div>
<div class="vm-overview-name">
<span class="state-badge ${getStateClass(vm.state)}">${isRunning ? '<i class="fas fa-circle"></i>' : '<i class="fas fa-circle-notch"></i>'}</span>
${escapeHtml(vm.name)}
</div>
<div class="vm-overview-cpu">CPU: ${stats ? (stats.cpu_usage || 0).toFixed(1) : '-'}% &nbsp; Mem: ${stats ? formatBytes(stats.memory_used) : '-'}</div>
${ips ? `<div class="vm-overview-ips">${ips}</div>` : ''}
</div>
</div>
<div class="vm-overview-actions" onclick="event.stopPropagation()">
${isRunning ? `
<button class="action-btn shutdown" title="Shutdown" onclick="vmAction('${escapeAttr(vm.name)}', 'shutdown')"><i class="fas fa-power-off"></i></button>
<button class="action-btn reboot" title="Reboot" onclick="vmAction('${escapeAttr(vm.name)}', 'reboot')"><i class="fas fa-sync"></i></button>
` : `
<button class="action-btn start" title="Start" onclick="vmAction('${escapeAttr(vm.name)}', 'start')"><i class="fas fa-play"></i></button>
`}
<button class="action-btn stop" title="Delete" onclick="vmAction('${escapeAttr(vm.name)}', 'delete')"><i class="fas fa-trash"></i></button>
</div>
</div>`;
}).join('');
}

function showVMDetail(name) {
loadVMDetail(name);
}

function initCharts() {
const ctx = document.getElementById('hostPerfChart').getContext('2d');
hostPerfChart = new Chart(ctx, {
type: 'line',
data: {
labels: [],
datasets: [
{
label: 'CPU %',
data: [],
borderColor: '#f5f5f5',
backgroundColor: 'rgba(245, 245, 245, 0.1)',
fill: true,
tension: 0.4
},
{
label: 'Memory %',
data: [],
borderColor: '#9e9e9e',
backgroundColor: 'rgba(158, 158, 158, 0.1)',
fill: true,
tension: 0.4
},
{
label: 'Net Rx (MB/s)',
data: [],
borderColor: '#616161',
backgroundColor: 'rgba(97, 97, 97, 0.1)',
fill: false,
tension: 0.4
}
]
},
options: {
responsive: true,
maintainAspectRatio: false,
animation: false,
plugins: {
legend: {
labels: { color: '#a0a0b0' }
}
},
scales: {
x: {
ticks: { color: '#a0a0b0', maxTicksLimit: 10 },
grid: { color: 'rgba(255,255,255,0.05)' }
},
y: {
beginAtZero: true,
ticks: { color: '#a0a0b0' },
grid: { color: 'rgba(255,255,255,0.05)' }
}
}
}
});

const vmCtx = document.getElementById('vmPerfChart').getContext('2d');
vmPerfChart = new Chart(vmCtx, {
type: 'line',
data: {
labels: [],
datasets: [
{
label: 'CPU %',
data: [],
borderColor: '#f5f5f5',
backgroundColor: 'rgba(245, 245, 245, 0.1)',
fill: true,
tension: 0.4
},
{
label: 'Memory %',
data: [],
borderColor: '#9e9e9e',
backgroundColor: 'rgba(158, 158, 158, 0.1)',
fill: true,
tension: 0.4
}
]
},
options: {
responsive: true,
maintainAspectRatio: false,
animation: false,
plugins: {
legend: {
labels: { color: '#a0a0b0' }
}
},
scales: {
x: {
ticks: { color: '#a0a0b0', maxTicksLimit: 10 },
grid: { color: 'rgba(255,255,255,0.05)' }
},
y: {
beginAtZero: true,
max: 100,
ticks: { color: '#a0a0b0' },
grid: { color: 'rgba(255,255,255,0.05)' }
}
}
}
});
}

function updateHostChart() {
if (!hostPerfChart) return;
const labels = state.hostPerfData.map(d => d.time.toLocaleTimeString());
const cpuData = state.hostPerfData.map(d => d.cpu);
const memData = state.hostPerfData.map(d => d.mem);
const netData = state.hostPerfData.map(d => d.net / (1024 * 1024));
hostPerfChart.data.labels = labels;
hostPerfChart.data.datasets[0].data = cpuData;
hostPerfChart.data.datasets[1].data = memData;
hostPerfChart.data.datasets[2].data = netData;
hostPerfChart.update('none');
}

function updateVMChart() {
if (!vmPerfChart || !state.currentVM) return;
const stats = state.vmStatsMap[state.currentVM];
if (!stats) return;

const now = new Date();
state.vmPerfData.push({
time: now,
cpu: stats.cpu_usage || 0,
mem: stats.memory_max > 0 ? (stats.memory_used / stats.memory_max * 100) : 0
});
if (state.vmPerfData.length > 30) {
state.vmPerfData.shift();
}

const labels = state.vmPerfData.map(d => d.time.toLocaleTimeString());
const cpuData = state.vmPerfData.map(d => d.cpu);
const memData = state.vmPerfData.map(d => d.mem);
vmPerfChart.data.labels = labels;
vmPerfChart.data.datasets[0].data = cpuData;
vmPerfChart.data.datasets[1].data = memData;
vmPerfChart.update('none');
}

setInterval(() => {
if (state.currentPage === 'vm-detail' && state.currentVM) {
updateVMChart();
}
}, 2000);

let wizardStep = 1;
const TOTAL_STEPS = 4;

function openWizard() {
wizardStep = 1;
document.getElementById('createVmModal').classList.add('active');
resetWizard();
loadNetworkOptions();
loadISOOptions();
}

function closeWizard() {
document.getElementById('createVmModal').classList.remove('active');
}

function resetWizard() {
document.getElementById('vmName').value = '';
document.getElementById('vmOS').value = 'linux';
document.getElementById('vmDescription').value = '';
document.getElementById('vmCores').value = '2';
document.getElementById('vmMemory').value = '2048';
document.getElementById('vmDisk').value = '20';
document.getElementById('vmStartOnCreate').checked = true;
setWizardStep(1);
}

function setWizardStep(step) {
wizardStep = step;

document.querySelectorAll('.wizard-panel').forEach((panel, i) => {
panel.classList.toggle('active', i + 1 === step);
});

document.querySelectorAll('.wizard-step').forEach((s, i) => {
s.classList.remove('active', 'completed');
const stepNum = i + 1;
if (stepNum < step) s.classList.add('completed');
if (stepNum === step) s.classList.add('active');
});

const prevBtn = document.getElementById('wizardPrevBtn');
const nextBtn = document.getElementById('wizardNextBtn');
const createBtn = document.getElementById('wizardCreateBtn');

prevBtn.disabled = step === 1;
nextBtn.classList.toggle('hidden', step === TOTAL_STEPS);
createBtn.classList.toggle('hidden', step !== TOTAL_STEPS);

if (step === TOTAL_STEPS) {
updateSummary();
}
}

function validateStep(step) {
if (step === 1) {
const name = document.getElementById('vmName').value.trim();
if (!name) {
showToast('VM name is required', 'warning');
return false;
}
if (!/^[a-zA-Z0-9_\-\.]+$/.test(name)) {
showToast('VM name can only contain letters, numbers, underscores, dashes and dots', 'warning');
return false;
}
return true;
}
if (step === 2) {
const cores = parseInt(document.getElementById('vmCores').value);
const memory = parseInt(document.getElementById('vmMemory').value);
const disk = parseInt(document.getElementById('vmDisk').value);
if (!cores || cores < 1 || cores > 64) {
showToast('CPU cores must be between 1 and 64', 'warning');
return false;
}
if (!memory || memory < 128 || memory > 65536) {
showToast('Memory must be between 128MB and 65536MB', 'warning');
return false;
}
if (!disk || disk < 1 || disk > 2048) {
showToast('Disk size must be between 1GB and 2048GB', 'warning');
return false;
}
return true;
}
return true;
}

async function loadNetworkOptions() {
const select = document.getElementById('vmNetwork');
select.innerHTML = '<option value="br0" selected>br0</option>';
try {
const networks = await apiFetch(API.networks);
const otherNetworks = networks.filter(n => n.name !== 'br0').map(n => `<option value="${escapeAttr(n.name)}">${escapeHtml(n.name)}</option>`).join('');
if (otherNetworks) {
select.innerHTML += otherNetworks;
}
} catch (e) {
console.error('Failed to load networks:', e);
}
}

async function loadISOOptions() {
try {
const isos = await apiFetch('/api/isos');
const select = document.getElementById('vmISO');
select.innerHTML = '<option value="">No ISO (boot from disk)</option>';
isos.forEach(iso => {
select.innerHTML += `<option value="${escapeAttr(iso.path)}">${escapeHtml(iso.name)} (${formatBytes(iso.size)})</option>`;
});
} catch (e) {
console.error('Failed to load ISOs:', e);
}
}

function setupISOUpload() {
const zone = document.getElementById('isoUploadZone');
const fileInput = document.getElementById('isoFileInput');

zone.addEventListener('click', () => fileInput.click());

zone.addEventListener('dragover', (e) => {
e.preventDefault();
zone.classList.add('dragover');
});

zone.addEventListener('dragleave', () => {
zone.classList.remove('dragover');
});

zone.addEventListener('drop', (e) => {
e.preventDefault();
zone.classList.remove('dragover');
const files = e.dataTransfer.files;
if (files.length > 0) {
uploadISO(files[0]);
}
});

fileInput.addEventListener('change', () => {
if (fileInput.files.length > 0) {
uploadISO(fileInput.files[0]);
}
});
}

async function uploadISO(file) {
if (!file.name.toLowerCase().endsWith('.iso')) {
showToast('Only .iso files are allowed', 'warning');
return;
}

const zone = document.getElementById('isoUploadZone');
const progress = document.getElementById('isoUploadProgress');
const progressBar = document.getElementById('isoUploadProgressBar');
const status = document.getElementById('isoUploadStatus');

zone.querySelector('i').style.display = 'none';
zone.querySelector('p').style.display = 'none';
zone.querySelectorAll('.text-muted').forEach(el => el.style.display = 'none');
progress.classList.remove('hidden');

const formData = new FormData();
formData.append('file', file);

const xhr = new XMLHttpRequest();
xhr.open('POST', '/api/isos/upload', true);

xhr.upload.onprogress = (e) => {
if (e.lengthComputable) {
const pct = Math.round((e.loaded / e.total) * 100);
progressBar.style.width = `${pct}%`;
status.textContent = `Uploading... ${pct}%`;
}
};

xhr.onload = () => {
if (xhr.status >= 200 && xhr.status < 300) {
const result = JSON.parse(xhr.responseText);
showToast(`ISO "${result.name}" uploaded successfully`, 'success');
progressBar.style.width = '100%';
status.textContent = 'Upload complete!';
setTimeout(() => {
progress.classList.add('hidden');
progressBar.style.width = '0%';
zone.querySelector('i').style.display = '';
zone.querySelector('p').style.display = '';
zone.querySelectorAll('.text-muted').forEach(el => el.style.display = '');
loadISOOptions();
}, 1500);
} else {
let msg = 'Upload failed';
try {
const err = JSON.parse(xhr.responseText);
msg = err.error || msg;
} catch (e) {}
showToast(`Failed: ${msg}`, 'error');
progress.classList.add('hidden');
zone.querySelector('i').style.display = '';
zone.querySelector('p').style.display = '';
zone.querySelectorAll('.text-muted').forEach(el => el.style.display = '');
}
};

xhr.onerror = () => {
showToast('Upload failed: network error', 'error');
progress.classList.add('hidden');
zone.querySelector('i').style.display = '';
zone.querySelector('p').style.display = '';
zone.querySelectorAll('.text-muted').forEach(el => el.style.display = '');
};

xhr.send(formData);
}

function updateSummary() {
const name = document.getElementById('vmName').value.trim();
const os = document.getElementById('vmOS').value;
const password = document.getElementById('vmPassword').value;
const desc = document.getElementById('vmDescription').value.trim();
const cores = document.getElementById('vmCores').value;
const memory = document.getElementById('vmMemory').value;
const disk = document.getElementById('vmDisk').value;
const network = document.getElementById('vmNetwork').value;
const isoSelect = document.getElementById('vmISO');
const iso = isoSelect.selectedOptions[0] ? isoSelect.selectedOptions[0].textContent : '';
const startOnCreate = document.getElementById('vmStartOnCreate').checked;

const summary = document.getElementById('createVmSummary');
summary.innerHTML = `
<h4><i class="fas fa-server"></i> VM Configuration Summary</h4>
<div class="summary-row"><span class="summary-label">Name</span><span class="summary-value">${escapeHtml(name || '-')}</span></div>
<div class="summary-row"><span class="summary-label">OS</span><span class="summary-value">${escapeHtml(os)}</span></div>
<div class="summary-row"><span class="summary-label">Root Password</span><span class="summary-value">${escapeHtml(password || 'root123')}</span></div>
<div class="summary-row"><span class="summary-label">Description</span><span class="summary-value">${escapeHtml(desc || '-')}</span></div>
<div class="summary-row"><span class="summary-label">CPU Cores</span><span class="summary-value">${cores}</span></div>
<div class="summary-row"><span class="summary-label">Memory</span><span class="summary-value">${formatMB(memory)}</span></div>
<div class="summary-row"><span class="summary-label">Disk</span><span class="summary-value">${disk} GB</span></div>
<div class="summary-row"><span class="summary-label">Network</span><span class="summary-value">${escapeHtml(network)}</span></div>
<div class="summary-row"><span class="summary-label">Installation ISO</span><span class="summary-value">${escapeHtml(iso || '-')}</span></div>
<div class="summary-row"><span class="summary-label">Start on Create</span><span class="summary-value">${startOnCreate ? '<i class="fas fa-check" style="color: var(--success)"></i>' : '<i class="fas fa-times" style="color: var(--danger)"></i>'}</span></div>
`;
}

async function createVM() {
const name = document.getElementById('vmName').value.trim();
const os = document.getElementById('vmOS').value;
const password = document.getElementById('vmPassword').value;
const cores = parseInt(document.getElementById('vmCores').value);
const memory = parseInt(document.getElementById('vmMemory').value);
const disk = parseInt(document.getElementById('vmDisk').value);
const network = document.getElementById('vmNetwork').value;
const iso = document.getElementById('vmISO').value;
const start = document.getElementById('vmStartOnCreate').checked;

const createBtn = document.getElementById('wizardCreateBtn');
		createBtn.disabled = true;
		createBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Creating...';

		try {
			const result = await apiFetch(API.vms, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name,
					password,
					os,
					vcpus: cores,
					memory,
					disk_size: disk,
					network,
					iso,
					start
				})
			});
			const steps = result.steps || [];
			const stepList = steps.length > 0
				? `<div style="text-align:left;margin-top:8px;font-size:0.85em;color:var(--text-secondary);">${steps.map(s => `<div>&bull; ${escapeHtml(s)}</div>`).join('')}</div>`
				: '';
			showToast(`<div><strong>VM "${name}" created successfully</strong>${stepList}</div>`, 'success');
			closeWizard();
			await refreshVMs();
			navigateTo('vms');
		} catch (err) {
			showToast(`Failed to create VM: ${err.message}`, 'error');
		} finally {
			createBtn.disabled = false;
			createBtn.innerHTML = '<i class="fas fa-check"></i> Create VM';
		}
}

function setupWizard() {
setupISOUpload();
document.getElementById('closeModalBtn').addEventListener('click', closeWizard);
document.getElementById('createVmModal').addEventListener('click', (e) => {
if (e.target === e.currentTarget) closeWizard();
});

document.getElementById('wizardPrevBtn').addEventListener('click', () => {
if (wizardStep > 1) setWizardStep(wizardStep - 1);
});

document.getElementById('wizardNextBtn').addEventListener('click', () => {
if (validateStep(wizardStep)) {
if (wizardStep < TOTAL_STEPS) setWizardStep(wizardStep + 1);
}
});

document.getElementById('wizardCreateBtn').addEventListener('click', createVM);

document.getElementById('createVmForm').addEventListener('submit', (e) => {
e.preventDefault();
if (wizardStep === TOTAL_STEPS) {
createVM();
} else if (validateStep(wizardStep)) {
setWizardStep(wizardStep + 1);
}
});
}

let confirmCallback = null;

function setupConfirmModal() {
document.getElementById('confirmCloseBtn').addEventListener('click', closeConfirm);
document.getElementById('confirmCancelBtn').addEventListener('click', closeConfirm);
document.getElementById('confirmModal').addEventListener('click', (e) => {
if (e.target === e.currentTarget) closeConfirm();
});
document.getElementById('confirmOkBtn').addEventListener('click', () => {
closeConfirm();
if (confirmCallback) confirmCallback();
});
}

function showConfirm(title, message, callback) {
document.getElementById('confirmTitle').textContent = title;
document.getElementById('confirmMessage').textContent = message;
confirmCallback = callback;
document.getElementById('confirmModal').classList.add('active');
}

function closeConfirm() {
document.getElementById('confirmModal').classList.remove('active');
confirmCallback = null;
}

function formatBytes(bytes) {
if (!bytes) return '0 B';
const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
const i = Math.floor(Math.log(bytes) / Math.log(1024));
return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatMB(mb) {
if (mb >= 1024) {
return `${(mb / 1024).toFixed(1)} GB`;
}
return `${mb} MB`;
}

function formatRate(bytesPerSec) {
if (bytesPerSec < 1024) return `${bytesPerSec.toFixed(1)} B/s`;
const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
const i = Math.floor(Math.log(bytesPerSec) / Math.log(1024));
return `${(bytesPerSec / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function escapeHtml(str) {
if (!str) return '';
const div = document.createElement('div');
div.textContent = str;
return div.innerHTML;
}

function escapeAttr(str) {
return escapeHtml(str).replace(/"/g, '\\u0022');
}

document.addEventListener('keydown', (e) => {
if (e.key === 'Escape') {
closeWizard();
closeConfirm();
}
});

document.addEventListener('DOMContentLoaded', init);