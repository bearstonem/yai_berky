// Helm GUI - Frontend Application

const API = '';  // same origin

// --- State ---
let allTools = [];
let allAgents = [];
let allSkills = [];
let allSessions = [];

// --- Page Navigation ---

function switchPage(page) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
  document.getElementById('page-' + page).classList.add('active');
  document.querySelector('[data-page="' + page + '"]').classList.add('active');

  // Close delegation panel when leaving agent page, hide flow tab
  if (page !== 'agent' && delegationPanelOpen) closeDelegationPanel();
  const flowTab = document.getElementById('deleg-toggle');
  if (flowTab) flowTab.style.display = page === 'agent' ? '' : 'none';

  // Load data for the page
  if (page === 'skills') loadSkills();
  if (page === 'sessions') loadSessions();
  if (page === 'settings') { loadConfig(); loadProviders(); renderThemePicker(); loadSavedFontSize(); }
  if (page === 'agents') loadAgentsList();
  if (page === 'agent') loadAgentProfiles();
}

// --- New chat / new task ---

function newChat() {
  activeSessionId = null;
  document.getElementById('chat-messages').innerHTML = '';
}

function newAgentTask() {
  activeSessionId = null;
  agentRunning = false;
  activeAgents = {};
  delegationEvents = {};
  updateStatusPanel();
  if (delegationPanelOpen) {
    renderDelegationTree();
    closeDelegationDetail();
  }
  document.getElementById('agent-messages').innerHTML = '';
  document.getElementById('agent-input').focus();
}

// --- Auto-resize textareas ---

function autoResize(el) {
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 200) + 'px';
}

document.querySelectorAll('.input-bar textarea').forEach(el => {
  el.addEventListener('input', () => autoResize(el));
});

// --- Input handling ---

function handleInputKey(e, mode) {
  if (e.key === 'Enter' && !e.shiftKey && !e.altKey) {
    e.preventDefault();
    if (mode === 'chat') sendChat();
    else if (mode === 'agent') sendAgent();
  }
}

// --- Chat ---

let chatStreaming = false;

function sendChat() {
  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message || chatStreaming) return;

  addMessage('chat-messages', message, 'user');
  input.value = '';
  autoResize(input);
  chatStreaming = true;

  const area = document.getElementById('chat-messages');
  // Show a streaming placeholder that accumulates raw text
  const placeholder = document.createElement('div');
  placeholder.className = 'msg msg-assistant msg-streaming';
  placeholder.textContent = '';
  area.appendChild(placeholder);
  let fullText = '';

  const chatBody = { message };
  if (activeSessionId) { chatBody.session_id = activeSessionId; activeSessionId = null; }

  fetchSSE('/api/chat', chatBody, {
    content: (data) => {
      fullText += data;
      // Show raw text while streaming (fast feedback)
      placeholder.textContent = stripThinkTagsSimple(fullText);
      area.scrollTop = area.scrollHeight;
    },
    done: () => {
      // Replace placeholder with properly rendered content
      placeholder.remove();
      renderFormattedResponse(area, fullText);
      chatStreaming = false;
      area.scrollTop = area.scrollHeight;
    },
    error: (data) => {
      placeholder.remove();
      if (fullText) renderFormattedResponse(area, fullText);
      addMessage('chat-messages', 'Error: ' + data, 'error');
      chatStreaming = false;
    }
  });
}

// --- Agent ---

let agentRunning = false;
let activeAgents = {}; // {agentId: {name, status, task}} — tracks all agents
let delegationEvents = {}; // {agentId: {name, status, task, events: [{type, data, ts}]}}
let delegationPanelOpen = false;
let delegTreeRafPending = false;

function updateStatusPanel() {
  const panel = document.getElementById('agent-status-panel');
  const ids = Object.keys(activeAgents);

  if (ids.length === 0 && !agentRunning) {
    panel.classList.add('hidden');
    return;
  }

  panel.classList.remove('hidden');
  let html = '';

  // Primary agent chip
  if (agentRunning) {
    const primaryDone = ids.length > 0 && ids.every(id => activeAgents[id].status !== 'active');
    const cls = primaryDone ? 'active' : 'active';
    html += '<div class="agent-status-chip active">' +
      '<span class="status-dot"></span> Primary Agent' +
      '<span class="status-label">orchestrating</span></div>';
  }

  // Sub-agent chips
  for (const id of ids) {
    const a = activeAgents[id];
    let cls = 'active';
    let label = 'working...';
    if (a.status === 'done') { cls = 'done'; label = 'done'; }
    else if (a.status === 'failed') { cls = 'failed'; label = 'failed'; }
    else if (a.status === 'waiting') { cls = 'waiting'; label = 'awaiting user'; }

    html += '<div class="agent-status-chip ' + cls + '">' +
      '<span class="status-dot"></span> ' + escapeHtml(a.name) +
      '<span class="status-label">' + label + '</span></div>';
  }

  panel.innerHTML = html;
}

// --- Delegation Flow Panel ---

function captureAgentEvent(agentId, type, data) {
  const id = agentId || '__primary__';
  if (!delegationEvents[id]) {
    delegationEvents[id] = {
      name: (data && data.agent_name) || (id === '__primary__' ? 'Primary Agent' : id),
      status: 'active',
      task: '',
      events: []
    };
  }
  delegationEvents[id].events.push({ type, data: data || {}, ts: Date.now() });
  scheduleDelegTreeRender();
}

function scheduleDelegTreeRender() {
  if (!delegationPanelOpen || delegTreeRafPending) return;
  delegTreeRafPending = true;
  requestAnimationFrame(() => {
    renderDelegationTree();
    delegTreeRafPending = false;
  });
}

function openDelegationPanel() {
  document.getElementById('delegation-panel').classList.remove('hidden');
  delegationPanelOpen = true;
  const tab = document.getElementById('deleg-toggle');
  if (tab) tab.style.right = '340px';
  renderDelegationTree();
}

function closeDelegationPanel() {
  document.getElementById('delegation-panel').classList.add('hidden');
  delegationPanelOpen = false;
  const tab = document.getElementById('deleg-toggle');
  if (tab) tab.style.right = '0';
}

function toggleDelegationPanel() {
  delegationPanelOpen ? closeDelegationPanel() : openDelegationPanel();
}

function renderDelegationTree() {
  const container = document.getElementById('deleg-tree');
  if (!container) return;

  const primary = delegationEvents['__primary__'];
  if (!primary) {
    container.innerHTML = '<div class="empty-state"><p>No activity yet. Run a task to see the delegation flow.</p></div>';
    return;
  }

  let html = renderDelegNode('__primary__', primary);

  const children = Object.entries(delegationEvents).filter(([id]) => id !== '__primary__');
  if (children.length > 0) {
    html += '<div class="deleg-children">';
    for (const [id, agent] of children) {
      html += renderDelegNode(id, agent);
    }
    html += '</div>';
  }

  container.innerHTML = html;

  // Update toggle button activity indicator
  const toggleBtn = document.getElementById('deleg-toggle');
  if (toggleBtn) {
    if (children.some(([, a]) => a.status === 'active')) {
      toggleBtn.classList.add('has-activity');
    } else {
      toggleBtn.classList.remove('has-activity');
    }
  }
}

function renderDelegNode(id, agent) {
  const statusClass = agent.status || 'active';
  const taskSnippet = truncate(agent.task || '', 60);
  const evCount = agent.events.length;
  const toolCalls = agent.events.filter(e => e.type === 'tool_call').length;
  const safeId = escapeHtml(id).replace(/'/g, "\\'");

  return '<div class="deleg-node ' + statusClass + '" onclick="showDelegationDetail(\'' + safeId + '\')">' +
    '<div class="deleg-node-name">' +
    '<span class="deleg-node-dot"></span>' +
    escapeHtml(agent.name) +
    '</div>' +
    (taskSnippet ? '<div class="deleg-node-task">' + escapeHtml(taskSnippet) + '</div>' : '') +
    '<div class="deleg-node-meta">' +
    '<span>' + evCount + ' events</span>' +
    (toolCalls > 0 ? '<span>' + toolCalls + ' tool calls</span>' : '') +
    '<span>' + statusClass + '</span>' +
    '</div>' +
    '</div>';
}

function showDelegationDetail(agentId) {
  const agent = delegationEvents[agentId];
  if (!agent) return;

  document.getElementById('deleg-tree').classList.add('hidden');
  const detail = document.getElementById('deleg-detail');
  detail.classList.remove('hidden');
  document.getElementById('deleg-detail-title').textContent = agent.name;

  const body = document.getElementById('deleg-detail-body');
  body.innerHTML = agent.events.map(ev => {
    let cls = '', label = '', content = '';
    switch (ev.type) {
      case 'tool_call':
        cls = 'tool-call';
        label = 'Tool Call';
        content = '<strong>' + escapeHtml(ev.data.name || '') + '</strong><br>' +
          '<code>' + escapeHtml(truncate(ev.data.arguments || '', 500)) + '</code>';
        break;
      case 'tool_result':
        cls = 'tool-result';
        label = 'Result';
        content = escapeHtml(truncate(ev.data.content || '', 500));
        break;
      case 'thinking':
        cls = 'thinking';
        label = 'Thinking';
        content = escapeHtml(truncate(ev.data.content || '', 300));
        break;
      case 'answer':
        cls = 'answer';
        label = 'Answer';
        content = escapeHtml(truncate(ev.data.content || '', 500));
        break;
      case 'sub_agent_start':
        cls = 'tool-call';
        label = 'Delegated';
        content = 'Delegated to <strong>' + escapeHtml(ev.data.agent_name || '') + '</strong>: ' + escapeHtml(truncate(ev.data.task || '', 200));
        break;
      case 'sub_agent_done':
        cls = 'answer';
        label = 'Sub-Agent Done';
        content = escapeHtml(ev.data.agent_name || '') + ': ' + escapeHtml(ev.data.status || '');
        break;
      case 'escalation':
        cls = 'escalation';
        label = 'Escalation';
        content = escapeHtml(ev.data.question || '');
        break;
      default:
        label = ev.type;
        content = JSON.stringify(ev.data).substring(0, 200);
    }
    const time = new Date(ev.ts).toLocaleTimeString();
    return '<div class="deleg-detail-item ' + cls + '">' +
      '<div class="detail-label">' + label + ' &middot; ' + time + '</div>' +
      content + '</div>';
  }).join('');
}

function closeDelegationDetail() {
  document.getElementById('deleg-detail').classList.add('hidden');
  document.getElementById('deleg-tree').classList.remove('hidden');
}

function sendAgent() {
  const input = document.getElementById('agent-input');
  const message = input.value.trim();
  if (!message || agentRunning) return;

  addMessage('agent-messages', message, 'user');
  input.value = '';
  autoResize(input);
  agentRunning = true;
  activeAgents = {};
  delegationEvents = { '__primary__': { name: 'Primary Agent', status: 'active', task: message, events: [] } };
  updateStatusPanel();
  if (delegationPanelOpen) renderDelegationTree();

  const area = document.getElementById('agent-messages');
  let currentMsg = null;
  const agentId = document.getElementById('agent-profile-select').value;
  const body = { message };
  if (agentId) body.agent_id = agentId;
  if (activeSessionId) { body.session_id = activeSessionId; activeSessionId = null; }

  fetchSSE('/api/agent', body, {
    thinking: (data) => {
      try {
        const info = JSON.parse(data);
        captureAgentEvent(info.agent_id, 'thinking', info);
        const tag = info.agent_name ? agentTag(info.agent_name) : '';
        if (tag) {
          const wrapper = document.createElement('div');
          wrapper.className = 'msg-sub-agent-group';
          wrapper.innerHTML = tag;
          area.appendChild(wrapper);
          renderFormattedResponse(wrapper, info.content || data, 'thinking');
        } else {
          renderFormattedResponse(area, info.content || data, 'thinking');
        }
      } catch(e) {
        renderFormattedResponse(area, data, 'thinking');
      }
      area.scrollTop = area.scrollHeight;
    },
    tool_call: (data) => {
      try {
        const tc = JSON.parse(data);
        captureAgentEvent(tc.agent_id, 'tool_call', tc);
        const el = document.createElement('div');
        el.className = 'msg msg-tool' + (tc.agent_id ? ' msg-sub-agent' : '');
        const tag = tc.agent_name ? agentTag(tc.agent_name) : '';
        el.innerHTML = tag +
          '<div class="tool-name">' + escapeHtml(tc.name) + '</div>' +
          '<div>' + escapeHtml(truncate(tc.arguments, 300)) + '</div>';
        area.appendChild(el);
        area.scrollTop = area.scrollHeight;
      } catch(e) {
        addMessage('agent-messages', '[tool] ' + data, 'tool');
      }
    },
    tool_result: (data) => {
      try {
        const info = JSON.parse(data);
        captureAgentEvent(info.agent_id, 'tool_result', info);
        const el = document.createElement('div');
        el.className = 'msg msg-tool' + (info.agent_id ? ' msg-sub-agent' : '');
        const tag = info.agent_name ? agentTag(info.agent_name) : '';
        el.innerHTML = tag +
          '<div class="tool-result">' + escapeHtml(truncate(info.content || data, 500)) + '</div>';
        area.appendChild(el);
      } catch(e) {
        const el = document.createElement('div');
        el.className = 'msg msg-tool';
        el.innerHTML = '<div class="tool-result">' + escapeHtml(truncate(data, 500)) + '</div>';
        area.appendChild(el);
      }
      area.scrollTop = area.scrollHeight;
    },
    answer: (data) => {
      try {
        const info = JSON.parse(data);
        captureAgentEvent(info.agent_id, 'answer', info);
        if (info.agent_name) {
          const wrapper = document.createElement('div');
          wrapper.className = 'msg-sub-agent-group';
          wrapper.innerHTML = agentTag(info.agent_name);
          area.appendChild(wrapper);
          renderFormattedResponse(wrapper, info.content || data);
        } else {
          renderFormattedResponse(area, info.content || data);
        }
      } catch(e) {
        renderFormattedResponse(area, data);
      }
      area.scrollTop = area.scrollHeight;
    },
    sub_agent_start: (data) => {
      try {
        const info = JSON.parse(data);
        activeAgents[info.agent_id] = { name: info.agent_name, status: 'active', task: info.task };
        // Track in delegation events + auto-open panel
        if (!delegationEvents[info.agent_id]) {
          delegationEvents[info.agent_id] = { name: info.agent_name, status: 'active', task: info.task, events: [] };
        }
        delegationEvents[info.agent_id].task = info.task;
        captureAgentEvent(info.agent_id, 'sub_agent_start', info);
        if (!delegationPanelOpen) openDelegationPanel();
        updateStatusPanel();
        const el = document.createElement('div');
        el.className = 'msg msg-sub-agent-start';
        el.innerHTML = '<span class="sub-agent-icon">&#x1F6F8;</span> ' +
          '<strong>' + escapeHtml(info.agent_name) + '</strong> started' +
          '<div class="sub-agent-task">' + escapeHtml(info.task) + '</div>';
        area.appendChild(el);
        area.scrollTop = area.scrollHeight;
      } catch(e) {}
    },
    sub_agent_done: (data) => {
      try {
        const info = JSON.parse(data);
        if (activeAgents[info.agent_id]) {
          activeAgents[info.agent_id].status = info.status === 'completed' ? 'done' : 'failed';
          updateStatusPanel();
        }
        if (delegationEvents[info.agent_id]) {
          delegationEvents[info.agent_id].status = info.status === 'completed' ? 'done' : 'failed';
        }
        captureAgentEvent(info.agent_id, 'sub_agent_done', info);
        const el = document.createElement('div');
        el.className = 'msg msg-sub-agent-done';
        el.innerHTML = '<span class="sub-agent-icon">&#x2705;</span> ' +
          '<strong>' + escapeHtml(info.agent_name) + '</strong> ' +
          escapeHtml(info.status);
        area.appendChild(el);
        area.scrollTop = area.scrollHeight;
      } catch(e) {}
    },
    escalation: (data) => {
      try {
        const info = JSON.parse(data);
        if (info.agent_id && activeAgents[info.agent_id]) {
          activeAgents[info.agent_id].status = 'waiting';
          updateStatusPanel();
        }
        if (info.agent_id && delegationEvents[info.agent_id]) {
          delegationEvents[info.agent_id].status = 'waiting';
        }
        captureAgentEvent(info.agent_id, 'escalation', info);
        const el = document.createElement('div');
        el.className = 'msg msg-escalation';
        const tag = info.agent_name ? agentTag(info.agent_name) : '';
        el.innerHTML = tag +
          '<div class="escalation-question">' + escapeHtml(info.question) + '</div>' +
          '<div class="escalation-input">' +
          '<input type="text" class="escalation-field" placeholder="Type your response..." ' +
          'onkeydown="if(event.key===\'Enter\')respondToEscalation(this)">' +
          '<button class="action-btn" onclick="respondToEscalation(this.previousElementSibling)">Respond</button>' +
          '</div>';
        area.appendChild(el);
        area.scrollTop = area.scrollHeight;
        el.querySelector('.escalation-field').focus();
      } catch(e) {}
    },
    error: (data) => {
      addMessage('agent-messages', 'Error: ' + data, 'error');
    },
    done: () => {
      agentRunning = false;
      if (delegationEvents['__primary__']) delegationEvents['__primary__'].status = 'done';
      updateStatusPanel();
      scheduleDelegTreeRender();
    }
  });
}

// --- SSE Helper (POST-based) ---

function fetchSSE(url, body, handlers) {
  fetch(API + url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(response => {
    if (!response.ok) {
      return response.text().then(t => {
        if (handlers.error) handlers.error(t);
        if (handlers.done) handlers.done();
      });
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let doneFired = false;

    function fireDone() {
      if (!doneFired) {
        doneFired = true;
        if (handlers.done) handlers.done();
      }
    }

    function read() {
      reader.read().then(({ done, value }) => {
        if (done) {
          fireDone();
          return;
        }

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop(); // keep incomplete line

        let currentEvent = '';
        let dataLines = [];
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7);
            dataLines = [];
          } else if (line.startsWith('data: ')) {
            dataLines.push(line.slice(6));
          } else if (line === '' && currentEvent) {
            // Empty line = end of event
            const data = dataLines.join('\n');
            if (currentEvent === 'done') {
              fireDone();
            } else if (handlers[currentEvent]) {
              handlers[currentEvent](data);
            }
            currentEvent = '';
            dataLines = [];
          }
        }

        read();
      });
    }

    read();
  }).catch(err => {
    if (handlers.error) handlers.error(err.message);
    if (handlers.done) handlers.done();
  });
}

// --- Message helpers ---

function addMessage(containerId, text, type) {
  const area = document.getElementById(containerId);
  const el = document.createElement('div');
  const classes = type.split(' ').map(t => 'msg-' + t).join(' ');
  el.className = 'msg ' + classes;
  el.textContent = text;
  area.appendChild(el);
  area.scrollTop = area.scrollHeight;
  return el;
}

// --- Skills ---

async function loadSkills() {
  const res = await fetch(API + '/api/skills');
  allSkills = await res.json();
  filterSkills();
}

function filterSkills() {
  const text = (document.getElementById('skills-filter-text').value || '').toLowerCase();
  const lang = document.getElementById('skills-filter-lang').value;
  const container = document.getElementById('skills-list');

  let filtered = allSkills;
  if (text) {
    filtered = filtered.filter(s =>
      (s.name || '').toLowerCase().includes(text) ||
      (s.description || '').toLowerCase().includes(text)
    );
  }
  if (lang) {
    filtered = filtered.filter(s => {
      const sl = (s.language || 'bash').toLowerCase();
      return sl === lang || sl.startsWith(lang);
    });
  }

  document.getElementById('skills-count').textContent =
    filtered.length === allSkills.length ? '' : filtered.length + ' of ' + allSkills.length;

  if (allSkills.length === 0) {
    container.innerHTML = '<div class="empty-state"><div class="empty-icon">&#x1F9E0;</div>#x1F527;</div>' +
      '<p>No skills yet. Create one or ask the agent to build a skill.</p></div>';
    return;
  }
  if (filtered.length === 0) {
    container.innerHTML = '<div class="empty-state"><p>No skills match your filter.</p></div>';
    return;
  }

  container.innerHTML = filtered.map(s => `
    <div class="card">
      <div class="card-title">
        <span>${escapeHtml(s.name)}</span>
        <span class="badge badge-lang">${escapeHtml(s.language || 'bash')}</span>
      </div>
      <div class="card-meta">Tool: <code>${escapeHtml('skill_' + s.name.toLowerCase().replace(/\s+/g, '_'))}</code></div>
      <div class="card-desc">${escapeHtml(s.description || 'No description')}</div>
      <div class="card-actions">
        <button class="card-btn" onclick="editSkill('${escapeHtml(s.name)}')">Edit</button>
        <button class="card-btn danger" onclick="deleteSkill('${escapeHtml(s.name)}')">Delete</button>
      </div>
    </div>
  `).join('');
}

function showCreateSkill() {
  document.getElementById('skill-modal-title').textContent = 'Create Skill';
  document.getElementById('skill-save-btn').textContent = 'Create';
  document.getElementById('skill-edit-name').value = '';
  document.getElementById('skill-name').value = '';
  document.getElementById('skill-name').disabled = false;
  document.getElementById('skill-desc').value = '';
  document.getElementById('skill-lang').value = 'bash';
  document.getElementById('skill-script').value = '';
  document.getElementById('skill-params').value = '';
  document.getElementById('skill-modal').classList.remove('hidden');
}

async function editSkill(name) {
  const res = await fetch(API + '/api/skills/' + encodeURIComponent(name));
  if (!res.ok) return;
  const s = await res.json();

  document.getElementById('skill-modal-title').textContent = 'Edit Skill';
  document.getElementById('skill-save-btn').textContent = 'Save';
  document.getElementById('skill-edit-name').value = s.name;
  document.getElementById('skill-name').value = s.name;
  document.getElementById('skill-name').disabled = true;
  document.getElementById('skill-desc').value = s.description || '';
  document.getElementById('skill-lang').value = s.language || 'bash';
  document.getElementById('skill-script').value = s.script || '';
  try {
    document.getElementById('skill-params').value =
      s.parameters ? JSON.stringify(s.parameters, null, 2) : '';
  } catch(e) {
    document.getElementById('skill-params').value = '';
  }
  document.getElementById('skill-modal').classList.remove('hidden');
}

async function saveSkill() {
  const editName = document.getElementById('skill-edit-name').value;
  const name = document.getElementById('skill-name').value.trim();
  const desc = document.getElementById('skill-desc').value.trim();
  const lang = document.getElementById('skill-lang').value;
  const script = document.getElementById('skill-script').value;
  const paramsStr = document.getElementById('skill-params').value.trim();

  if (!name || !script) {
    alert('Name and script are required');
    return;
  }

  let params = null;
  if (paramsStr) {
    try {
      params = JSON.parse(paramsStr);
    } catch (e) {
      alert('Invalid JSON in parameters');
      return;
    }
  }

  let res;
  if (editName) {
    // Update existing skill
    res = await fetch(API + '/api/skills/' + encodeURIComponent(editName), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ description: desc, language: lang, script, parameters: params })
    });
  } else {
    // Create new skill
    res = await fetch(API + '/api/skills', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, description: desc, language: lang, script, parameters: params })
    });
  }

  if (res.ok) {
    hideModal('skill-modal');
    loadSkills();
  } else {
    const err = await res.json();
    alert('Error: ' + (err.error || 'Unknown error'));
  }
}

async function deleteSkill(name) {
  if (!confirm('Delete skill "' + name + '"?')) return;
  const res = await fetch(API + '/api/skills/' + encodeURIComponent(name), { method: 'DELETE' });
  if (res.ok) {
    loadSkills();
  } else {
    const err = await res.json();
    alert('Error: ' + (err.error || 'Unknown error'));
  }
}

// --- Sessions ---

async function loadSessions() {
  const res = await fetch(API + '/api/sessions');
  allSessions = await res.json();
  filterSessions();
}

function filterSessions() {
  const text = (document.getElementById('sessions-filter-text').value || '').toLowerCase();
  const mode = document.getElementById('sessions-filter-mode').value;
  const container = document.getElementById('sessions-list');

  let filtered = allSessions;
  if (text) {
    filtered = filtered.filter(s =>
      (s.summary || '').toLowerCase().includes(text) ||
      (s.id || '').toLowerCase().includes(text)
    );
  }
  if (mode) {
    filtered = filtered.filter(s => s.mode === mode);
  }

  document.getElementById('sessions-count').textContent =
    filtered.length === allSessions.length ? '' : filtered.length + ' of ' + allSessions.length;

  if (allSessions.length === 0) {
    container.innerHTML = '<div class="empty-state"><div class="empty-icon">&#x1F4C1;</div>#x1F4DC;</div>' +
      '<p>No sessions yet. Start a conversation to create one.</p></div>';
    return;
  }
  if (filtered.length === 0) {
    container.innerHTML = '<div class="empty-state"><p>No sessions match your filter.</p></div>';
    return;
  }

  container.innerHTML = filtered.map(s => `
    <div class="card" style="cursor:pointer" onclick="viewSession('${escapeHtml(s.id)}')">
      <div class="card-title">
        <span class="badge badge-mode">${escapeHtml(s.mode)}</span>
        <span>${escapeHtml(s.summary || 'Untitled')}</span>
      </div>
      <div class="card-meta">${escapeHtml(s.id)} &middot; ${s.messages} messages &middot; ${formatDate(s.updated_at)}</div>
      <div class="card-actions">
        <button class="card-btn" onclick="event.stopPropagation(); resumeSession('${escapeHtml(s.id)}', '${escapeHtml(s.mode)}')">Resume</button>
        <button class="card-btn danger" onclick="event.stopPropagation(); deleteSession('${escapeHtml(s.id)}')">Delete</button>
      </div>
    </div>
  `).join('');
}

let activeSessionId = null; // tracks the session being resumed

async function resumeSession(id, mode) {
  const res = await fetch(API + '/api/sessions/' + id);
  if (!res.ok) return;
  const sess = await res.json();

  activeSessionId = id;

  // Switch to the appropriate page
  if (mode === 'agent') {
    switchPage('agent');
    const area = document.getElementById('agent-messages');
    area.innerHTML = '';
    // Render previous messages
    renderSessionHistory(area, sess.messages);
    // Add a resume indicator
    const indicator = document.createElement('div');
    indicator.className = 'msg msg-resume-indicator';
    indicator.innerHTML = '&#x1F4CB; Resumed session <strong>' + escapeHtml(id) + '</strong> &mdash; ' +
      escapeHtml(sess.summary || 'untitled') + ' (' + sess.messages.length + ' messages)';
    area.appendChild(indicator);
    area.scrollTop = area.scrollHeight;
  } else {
    switchPage('chat');
    const area = document.getElementById('chat-messages');
    area.innerHTML = '';
    renderSessionHistory(area, sess.messages);
    const indicator = document.createElement('div');
    indicator.className = 'msg msg-resume-indicator';
    indicator.innerHTML = '&#x1F4CB; Resumed session <strong>' + escapeHtml(id) + '</strong> &mdash; ' +
      escapeHtml(sess.summary || 'untitled') + ' (' + sess.messages.length + ' messages)';
    area.appendChild(indicator);
    area.scrollTop = area.scrollHeight;
  }
}

function renderSessionHistory(container, messages) {
  for (const m of messages) {
    if (m.role === 'system') continue; // skip system prompt
    if (m.role === 'user') {
      addMessageTo(container, m.content, 'user');
    } else if (m.role === 'tool') {
      const el = document.createElement('div');
      el.className = 'msg msg-tool';
      let label = m.tool_call_id || 'tool';
      el.innerHTML = '<div class="tool-result">' + escapeHtml(truncate(m.content, 300)) + '</div>';
      container.appendChild(el);
    } else if (m.role === 'assistant') {
      if (m.tool_calls && m.tool_calls.length > 0) {
        // Show tool calls
        for (const tc of m.tool_calls) {
          const el = document.createElement('div');
          el.className = 'msg msg-tool';
          el.innerHTML = '<div class="tool-name">' + escapeHtml(tc.name) + '</div>' +
            '<div>' + escapeHtml(truncate(tc.arguments, 200)) + '</div>';
          container.appendChild(el);
        }
        if (m.content) {
          renderFormattedResponse(container, m.content, 'thinking');
        }
      } else if (m.content) {
        renderFormattedResponse(container, m.content);
      }
    }
  }
}

function addMessageTo(container, text, type) {
  const el = document.createElement('div');
  el.className = 'msg msg-' + type;
  el.textContent = text;
  container.appendChild(el);
}

async function viewSession(id) {
  const res = await fetch(API + '/api/sessions/' + id);
  if (!res.ok) return;
  const sess = await res.json();

  document.getElementById('session-modal-title').textContent =
    (sess.summary || 'Session') + ' (' + sess.mode + ')';

  const container = document.getElementById('session-messages');
  container.innerHTML = sess.messages.map(m => {
    const type = m.role === 'user' ? 'user' : m.role === 'tool' ? 'tool' : 'assistant';
    let content = m.content || '';
    if (m.tool_calls && m.tool_calls.length > 0) {
      content += m.tool_calls.map(tc =>
        '\n[tool_call: ' + tc.name + '(' + truncate(tc.arguments, 100) + ')]'
      ).join('');
    }
    return `<div class="session-msg session-msg-${type}">
      <div class="session-msg-label">${escapeHtml(m.role)}${m.tool_call_id ? ' (' + m.tool_call_id + ')' : ''}</div>
      ${escapeHtml(truncate(content, 1000))}
    </div>`;
  }).join('');

  document.getElementById('session-modal').classList.remove('hidden');
}

async function deleteSession(id) {
  if (!confirm('Delete session ' + id + '?')) return;
  await fetch(API + '/api/sessions/' + id, { method: 'DELETE' });
  loadSessions();
}

// --- Agents ---

async function loadTools() {
  if (allTools.length > 0) return;
  try {
    const res = await fetch(API + '/api/tools');
    allTools = await res.json();
  } catch(e) { /* ignore */ }
}

async function loadAgentsList() {
  await loadTools();
  const res = await fetch(API + '/api/agents');
  const agents = await res.json();
  allAgents = agents;
  const container = document.getElementById('agents-list');

  if (agents.length === 0) {
    container.innerHTML = '<div class="empty-state"><div class="empty-icon">&#x1F47E;#x1F6A2;</div>' +
      '<p>No custom agents yet. Create one to get started.</p></div>';
    return;
  }

  container.innerHTML = agents.map(a => {
    const toolCount = a.tools && a.tools.length > 0
      ? a.tools.length + ' tools'
      : 'all tools';
    return `
    <div class="card">
      <div class="card-title">
        <span>&#x1F47E;#x1F6A2;</span>
        <span>${escapeHtml(a.name)}</span>
        ${a.model ? '<span class="badge badge-lang">' + escapeHtml(a.model) + '</span>' : ''}
      </div>
      <div class="card-meta">${escapeHtml(a.id)} &middot; ${toolCount} &middot; ${formatDate(a.updated_at)}</div>
      <div class="card-desc">${escapeHtml(a.description || 'No description')}</div>
      <div class="card-desc" style="font-size:12px;color:var(--text-dimmer);max-height:60px;overflow:hidden">${escapeHtml(truncate(a.system_prompt || '', 200))}</div>
      <div class="card-actions">
        <button class="card-btn" onclick="editAgent('${escapeHtml(a.id)}')">Edit</button>
        <button class="card-btn danger" onclick="deleteAgent('${escapeHtml(a.id)}')">Delete</button>
      </div>
    </div>`;
  }).join('');
}

async function showCreateAgent() {
  await loadTools();
  document.getElementById('agent-modal-title').textContent = 'Create Agent';
  document.getElementById('agent-save-btn').textContent = 'Create';
  document.getElementById('agent-edit-id').value = '';
  document.getElementById('agent-name').value = '';
  document.getElementById('agent-desc').value = '';
  document.getElementById('agent-model').value = '';
  document.getElementById('agent-prompt').value = '';
  renderToolsChecklist([]);
  document.getElementById('agent-modal').classList.remove('hidden');
}

async function editAgent(id) {
  await loadTools();
  const res = await fetch(API + '/api/agents/' + id);
  if (!res.ok) return;
  const a = await res.json();

  document.getElementById('agent-modal-title').textContent = 'Edit Agent';
  document.getElementById('agent-save-btn').textContent = 'Save';
  document.getElementById('agent-edit-id').value = a.id;
  document.getElementById('agent-name').value = a.name;
  document.getElementById('agent-desc').value = a.description || '';
  document.getElementById('agent-model').value = a.model || '';
  document.getElementById('agent-prompt').value = a.system_prompt || '';
  renderToolsChecklist(a.tools || []);
  document.getElementById('agent-modal').classList.remove('hidden');
}

function renderToolsChecklist(selectedTools) {
  const container = document.getElementById('agent-tools-checklist');
  const selected = new Set(selectedTools);

  container.innerHTML = allTools.map(t => `
    <label class="tool-check">
      <input type="checkbox" value="${escapeHtml(t.name)}" ${selected.has(t.name) ? 'checked' : ''}>
      <div>
        <div class="tool-check-name">${escapeHtml(t.name)}</div>
        <div class="tool-check-desc">${escapeHtml(truncate(t.description, 80))}</div>
      </div>
    </label>
  `).join('');
}

function getSelectedTools() {
  const checks = document.querySelectorAll('#agent-tools-checklist input[type="checkbox"]:checked');
  return Array.from(checks).map(c => c.value);
}

async function saveAgent() {
  const editId = document.getElementById('agent-edit-id').value;
  const name = document.getElementById('agent-name').value.trim();
  const desc = document.getElementById('agent-desc').value.trim();
  const model = document.getElementById('agent-model').value.trim();
  const prompt = document.getElementById('agent-prompt').value;
  const tools = getSelectedTools();

  if (!name) {
    alert('Name is required');
    return;
  }

  const body = {
    name,
    description: desc,
    system_prompt: prompt,
    tools: tools,
    model: model || undefined
  };

  let res;
  if (editId) {
    res = await fetch(API + '/api/agents/' + editId, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  } else {
    res = await fetch(API + '/api/agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  if (res.ok) {
    hideModal('agent-modal');
    loadAgentsList();
  } else {
    const err = await res.json();
    alert('Error: ' + (err.error || 'Unknown error'));
  }
}

async function deleteAgent(id) {
  if (!confirm('Delete this agent?')) return;
  await fetch(API + '/api/agents/' + id, { method: 'DELETE' });
  loadAgentsList();
}

// --- Agent Profile Selector (on Agent run page) ---

async function loadAgentProfiles() {
  const res = await fetch(API + '/api/agents');
  const agents = await res.json();
  allAgents = agents;
  const select = document.getElementById('agent-profile-select');
  const currentVal = select.value;

  // Keep "Default Agent" option, rebuild the rest
  select.innerHTML = '<option value="">Primary Agent</option>';
  agents.forEach(a => {
    const opt = document.createElement('option');
    opt.value = a.id;
    opt.textContent = a.name;
    select.appendChild(opt);
  });

  // Restore selection
  if (currentVal) select.value = currentVal;
}

function onAgentProfileChange() {
  const id = document.getElementById('agent-profile-select').value;
  const info = document.getElementById('agent-profile-info');
  if (!id) {
    info.classList.add('hidden');
    return;
  }
  const a = allAgents.find(a => a.id === id);
  if (a) {
    const toolInfo = a.tools && a.tools.length > 0
      ? a.tools.length + ' tools: ' + a.tools.join(', ')
      : 'all tools';
    info.innerHTML = '<strong>' + escapeHtml(a.name) + '</strong> &mdash; ' +
      escapeHtml(a.description || 'No description') +
      '<br><small>' + escapeHtml(toolInfo) +
      (a.model ? ' &middot; model: ' + escapeHtml(a.model) : '') + '</small>';
    info.classList.remove('hidden');
  }
}

// --- Config & Providers ---

let currentConfig = {};

async function loadConfig() {
  const [cfgRes, provRes] = await Promise.all([
    fetch(API + '/api/config'),
    fetch(API + '/api/providers')
  ]);
  currentConfig = await cfgRes.json();
  const providers = await provRes.json();
  const cfg = currentConfig;

  const container = document.getElementById('settings-form');
  const providerOpts = providers.map(p =>
    `<option value="${escapeHtml(p.id)}" ${cfg.provider === p.id ? 'selected' : ''}>${escapeHtml(p.name)}</option>`
  ).join('');

  const modeOpts = ['exec', 'chat', 'agent'].map(m =>
    `<option value="${m}" ${(cfg.default_prompt_mode || 'exec') === m ? 'selected' : ''}>${m}</option>`
  ).join('');

  const permOpts = ['read-only', 'workspace-write', 'full-access'].map(m =>
    `<option value="${m}" ${(cfg.permission_mode || 'workspace-write') === m ? 'selected' : ''}>${m}</option>`
  ).join('');

  container.innerHTML = `
    <div class="form-group">
      <label>Provider</label>
      <select id="cfg-provider" onchange="onProviderChange()">
        ${providerOpts}
      </select>
    </div>
    <div class="form-group">
      <label>API Key</label>
      <input type="password" id="cfg-api-key" value="${escapeHtml(cfg.api_key || '')}" placeholder="Your API key">
      <p class="form-hint">Leave unchanged to keep current key</p>
    </div>
    <div class="form-group">
      <label>Model</label>
      <input type="text" id="cfg-model" value="${escapeHtml(cfg.model || '')}" placeholder="e.g. gpt-4o-mini">
    </div>
    <div class="form-group">
      <label>Base URL</label>
      <input type="text" id="cfg-base-url" value="${escapeHtml(cfg.base_url || '')}" placeholder="Leave empty for default">
      <p class="form-hint">Auto-set for known providers. Override for custom endpoints.</p>
    </div>
    <div class="form-group">
      <label>Proxy</label>
      <input type="text" id="cfg-proxy" value="${escapeHtml(cfg.proxy || '')}" placeholder="http://proxy:port">
    </div>
    <div class="form-group">
      <label>Temperature</label>
      <input type="number" id="cfg-temperature" value="${cfg.temperature || 0.2}" min="0" max="2" step="0.1">
    </div>
    <div class="form-group">
      <label>Max Tokens</label>
      <input type="number" id="cfg-max-tokens" value="${cfg.max_tokens || 2000}" min="100" max="128000" step="100">
    </div>
    <div class="form-group">
      <label>Default Prompt Mode</label>
      <select id="cfg-mode">${modeOpts}</select>
    </div>
    <div class="form-group">
      <label>Permission Mode</label>
      <select id="cfg-permission">${permOpts}</select>
    </div>
    <div class="form-group">
      <label>Auto Execute Agent Tools</label>
      <select id="cfg-auto-exec">
        <option value="false" ${!cfg.auto_execute ? 'selected' : ''}>No (confirm each tool)</option>
        <option value="true" ${cfg.auto_execute ? 'selected' : ''}>Yes (yolo mode)</option>
      </select>
    </div>
    <div class="form-group">
      <label>Allow Sudo</label>
      <select id="cfg-sudo">
        <option value="false" ${!cfg.allow_sudo ? 'selected' : ''}>No</option>
        <option value="true" ${cfg.allow_sudo ? 'selected' : ''}>Yes</option>
      </select>
    </div>
    <div class="form-group full-width">
      <label>User Preferences</label>
      <textarea id="cfg-preferences" rows="3" placeholder="Free-text preferences appended to the system prompt...">${escapeHtml(cfg.preferences || '')}</textarea>
    </div>
    <div class="settings-actions">
      <button class="action-btn" onclick="saveSettings()">Save Settings</button>
      <button class="cancel-btn" onclick="loadConfig()">Reset</button>
    </div>
  `;

  document.getElementById('settings-status').classList.add('hidden');

  // Update sidebar badge
  document.getElementById('provider-badge').textContent =
    cfg.provider + ' / ' + cfg.model;
}

function onProviderChange() {
  // No-op for now; user can manually set model/url
}

async function saveSettings() {
  const body = {
    provider: document.getElementById('cfg-provider').value,
    api_key: document.getElementById('cfg-api-key').value,
    model: document.getElementById('cfg-model').value,
    base_url: document.getElementById('cfg-base-url').value,
    proxy: document.getElementById('cfg-proxy').value,
    temperature: parseFloat(document.getElementById('cfg-temperature').value) || 0.2,
    max_tokens: parseInt(document.getElementById('cfg-max-tokens').value) || 2000,
    default_prompt_mode: document.getElementById('cfg-mode').value,
    permission_mode: document.getElementById('cfg-permission').value,
    auto_execute: document.getElementById('cfg-auto-exec').value === 'true',
    allow_sudo: document.getElementById('cfg-sudo').value === 'true',
    preferences: document.getElementById('cfg-preferences').value,
  };

  const statusEl = document.getElementById('settings-status');

  try {
    const res = await fetch(API + '/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });

    if (res.ok) {
      statusEl.textContent = 'Settings saved successfully.';
      statusEl.className = 'settings-status success';
      statusEl.classList.remove('hidden');
      // Reload to reflect new values
      await loadConfig();
      setTimeout(() => statusEl.classList.add('hidden'), 3000);
    } else {
      const err = await res.json();
      statusEl.textContent = 'Error: ' + (err.error || 'Unknown error');
      statusEl.className = 'settings-status error';
      statusEl.classList.remove('hidden');
    }
  } catch(e) {
    statusEl.textContent = 'Error: ' + e.message;
    statusEl.className = 'settings-status error';
    statusEl.classList.remove('hidden');
  }
}

async function loadProviders() {
  const res = await fetch(API + '/api/providers');
  const providers = await res.json();
  const container = document.getElementById('providers-list');

  container.innerHTML = providers.map(p => `
    <div class="card" style="cursor:pointer" onclick="selectProvider('${escapeHtml(p.id)}', '${escapeHtml(p.default_model)}')">
      <div class="card-title">${escapeHtml(p.name)}</div>
      <div class="card-meta">${escapeHtml(p.id)} &middot; Default: <code>${escapeHtml(p.default_model)}</code></div>
      <div class="card-desc">${p.needs_api_key ? 'Requires API key' : 'No API key needed (local)'}</div>
    </div>
  `).join('');
}

function selectProvider(id, defaultModel) {
  const provSelect = document.getElementById('cfg-provider');
  const modelInput = document.getElementById('cfg-model');
  if (provSelect) provSelect.value = id;
  if (modelInput) modelInput.value = defaultModel;
  // Scroll to top of settings
  document.getElementById('settings-form').scrollIntoView({ behavior: 'smooth' });
}

// --- Memory Stats ---

async function loadMemoryStats() {
  try {
    const res = await fetch(API + '/api/memory/stats');
    const stats = await res.json();
    const el = document.getElementById('memory-stats');
    if (stats.available) {
      el.textContent = `Memory: ${stats.messages}m / ${stats.skills}s / ${stats.sessions}x`;
    } else {
      el.textContent = 'Memory: offline';
    }
  } catch (e) {
    // ignore
  }
}

// --- Modal ---

function hideModal(id) {
  document.getElementById(id).classList.add('hidden');
}

// Close modals on backdrop click
document.querySelectorAll('.modal').forEach(modal => {
  modal.addEventListener('click', (e) => {
    if (e.target === modal) modal.classList.add('hidden');
  });
});

// --- Utilities ---

async function respondToEscalation(inputEl) {
  const response = inputEl.value.trim();
  if (!response) return;

  // Disable the input
  const container = inputEl.closest('.msg-escalation');
  container.querySelector('.escalation-input').innerHTML =
    '<em>You responded: ' + escapeHtml(response) + '</em>';

  try {
    await fetch(API + '/api/agent/respond', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ response })
    });
  } catch(e) {
    console.error('Failed to send escalation response:', e);
  }
}

function agentTag(name) {
  return '<span class="sub-agent-tag">&#x1F6F8; ' + escapeHtml(name) + '</span>';
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function truncate(str, max) {
  if (!str) return '';
  if (str.length <= max) return str;
  return str.slice(0, max) + '...';
}

function formatDate(dateStr) {
  if (!dateStr) return '';
  const d = new Date(dateStr);
  const now = new Date();
  const diff = now - d;

  if (diff < 60000) return 'just now';
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
  if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
  if (diff < 604800000) return Math.floor(diff / 86400000) + 'd ago';
  return d.toLocaleDateString();
}

// --- Formatted response rendering ---

// Render a complete response, splitting think blocks from content.
// msgClass defaults to 'assistant' for normal messages, 'thinking' for agent thinking.
function renderFormattedResponse(container, text, msgClass) {
  msgClass = msgClass || 'assistant';

  // Handle case where <think> has no </think> — treat everything after
  // the first double-newline (or end) as the actual content
  text = fixUnclosedThinkTags(text);

  // Split on <think>...</think> blocks
  const parts = text.split(/(<think>[\s\S]*?<\/think>)/g);

  for (const part of parts) {
    if (!part.trim()) continue;

    const thinkMatch = part.match(/^<think>([\s\S]*)<\/think>$/);
    if (thinkMatch) {
      const el = document.createElement('details');
      el.className = 'msg msg-think-block';
      el.innerHTML = '<summary class="think-summary">Thinking...</summary>' +
        '<div class="think-content">' + escapeHtml(thinkMatch[1].trim()) + '</div>';
      container.appendChild(el);
    } else {
      const el = document.createElement('div');
      el.className = 'msg msg-' + msgClass;
      el.innerHTML = renderFormattedText(part.trim());
      container.appendChild(el);
    }
  }
}

// Fix unclosed <think> tags by inserting </think> at the first double-newline
// or at the end if no double-newline is found.
function fixUnclosedThinkTags(text) {
  let result = text;
  let searchFrom = 0;

  while (true) {
    const openIdx = result.indexOf('<think>', searchFrom);
    if (openIdx === -1) break;

    const afterOpen = openIdx + 7;
    const closeIdx = result.indexOf('</think>', afterOpen);

    if (closeIdx === -1) {
      // No closing tag — find the boundary
      const dblNewline = result.indexOf('\n\n', afterOpen);
      if (dblNewline !== -1) {
        // Insert </think> before the double newline
        result = result.substring(0, dblNewline) + '</think>' + result.substring(dblNewline);
      } else {
        // No double newline — close at end
        result = result + '</think>';
      }
    }

    searchFrom = openIdx + 1;
    // Safety: prevent infinite loop
    if (searchFrom > result.length) break;
  }

  return result;
}

// Strip think tags for display during streaming (simple version, no HTML)
function stripThinkTagsSimple(text) {
  // Remove complete think blocks
  text = text.replace(/<think>[\s\S]*?<\/think>/g, '');
  // Remove unclosed think tag and everything after it
  const openIdx = text.indexOf('<think>');
  if (openIdx !== -1) {
    text = text.substring(0, openIdx);
  }
  return text.trim();
}

// --- Think-aware streamer ---
// Streams content into a container, rendering <think> blocks into collapsible
// "thinking" elements and normal text into assistant message bubbles.

function createThinkStreamer(container) {
  let full = '';
  let buffer = '';
  let insideThink = false;
  let thinkEl = null;
  let msgEl = null;
  let msgRawText = '';     // raw text for current message (for post-processing)

  function ensureMsgEl() {
    if (!msgEl) {
      msgEl = document.createElement('div');
      msgEl.className = 'msg msg-assistant msg-streaming';
      msgRawText = '';
      container.appendChild(msgEl);
    }
    return msgEl;
  }

  function ensureThinkEl() {
    if (!thinkEl) {
      thinkEl = document.createElement('details');
      thinkEl.className = 'msg msg-think-block';
      thinkEl.innerHTML = '<summary class="think-summary">Thinking...</summary>';
      const content = document.createElement('div');
      content.className = 'think-content';
      thinkEl.appendChild(content);
      container.appendChild(thinkEl);
    }
    return thinkEl.querySelector('.think-content');
  }

  function finalizeMsgEl() {
    if (msgEl && msgRawText) {
      msgEl.innerHTML = renderFormattedText(msgRawText);
      msgEl.classList.remove('msg-streaming');
      msgEl = null;
      msgRawText = '';
    } else if (msgEl) {
      msgEl.classList.remove('msg-streaming');
      msgEl = null;
    }
  }

  return {
    push(data) {
      full += data;
      buffer += data;

      while (true) {
        if (!insideThink) {
          const openIdx = buffer.indexOf('<think>');
          if (openIdx === -1) break;
          const before = buffer.substring(0, openIdx);
          if (before) {
            ensureMsgEl().textContent += before;
            msgRawText += before;
          }
          buffer = buffer.substring(openIdx + 7);
          insideThink = true;
          finalizeMsgEl();
          thinkEl = null;
        }
        if (insideThink) {
          // Look for explicit </think> close tag
          const closeIdx = buffer.indexOf('</think>');
          if (closeIdx !== -1) {
            const thinkContent = buffer.substring(0, closeIdx);
            if (thinkContent) {
              ensureThinkEl().textContent += thinkContent;
            }
            buffer = buffer.substring(closeIdx + 8);
            insideThink = false;
            thinkEl = null;
            continue; // keep processing buffer
          }

          // Heuristic: some models never send </think>.
          // Detect end of thinking by double newline (paragraph break)
          // which signals the model switching from internal reasoning to response.
          const dblNewline = buffer.indexOf('\n\n');
          if (dblNewline !== -1 && buffer.length > dblNewline + 2) {
            // Flush think content up to the break
            const thinkContent = buffer.substring(0, dblNewline);
            if (thinkContent) {
              ensureThinkEl().textContent += thinkContent;
            }
            buffer = buffer.substring(dblNewline + 2);
            insideThink = false;
            thinkEl = null;
            continue; // process remaining buffer as normal text
          }

          // Still accumulating think content — flush what we have
          if (buffer) {
            ensureThinkEl().textContent += buffer;
            buffer = '';
            container.scrollTop = container.scrollHeight;
          }
          break;
        }
      }

      if (!insideThink && buffer) {
        ensureMsgEl().textContent += buffer;
        msgRawText += buffer;
        buffer = '';
        container.scrollTop = container.scrollHeight;
      }
    },

    finish() {
      if (buffer) {
        if (insideThink) {
          ensureThinkEl().textContent += buffer;
        } else {
          ensureMsgEl().textContent += buffer;
          msgRawText += buffer;
        }
        buffer = '';
      }
      finalizeMsgEl();
    },

    fullText() {
      return full;
    }
  };
}

// Render text with code blocks and inline code as formatted HTML
function renderFormattedText(text) {
  const parts = [];
  let remaining = text;

  while (remaining) {
    // Find next fenced code block
    const fenceMatch = remaining.match(/```([a-zA-Z_]*)\n([\s\S]*?)```/);
    if (!fenceMatch) {
      parts.push(renderInlineFormatting(escapeHtml(remaining)));
      break;
    }

    const idx = remaining.indexOf(fenceMatch[0]);
    // Text before the code block
    if (idx > 0) {
      parts.push(renderInlineFormatting(escapeHtml(remaining.substring(0, idx))));
    }

    // The code block
    const lang = fenceMatch[1] || '';
    const code = fenceMatch[2];
    parts.push(
      '<pre>' +
      (lang ? '<code-lang>' + escapeHtml(lang) + '</code-lang>' : '') +
      '<code>' + escapeHtml(code) + '</code></pre>'
    );

    remaining = remaining.substring(idx + fenceMatch[0].length);
  }

  return parts.join('');
}

// Render inline formatting: `code`, **bold**, *italic*
function renderInlineFormatting(html) {
  // Inline code
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  // Bold
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  // Italic (single *)
  html = html.replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, '<em>$1</em>');
  // Preserve newlines
  html = html.replace(/\n/g, '<br>');
  return html;
}

// --- Builder (AI-assisted creation) ---

let builderType = ''; // 'skill' or 'agent'
let builderMessages = []; // conversation history
let builderStreaming = false;
let builderResult = null; // parsed definition when detected

function openSkillBuilder() {
  builderType = 'skill';
  builderMessages = [];
  builderResult = null;
  document.getElementById('builder-title').textContent = 'AI Skill Builder';
  document.getElementById('builder-input').placeholder = 'Describe the skill you want to create...';
  resetBuilderUI();
  document.getElementById('builder-modal').classList.remove('hidden');
  document.getElementById('builder-input').focus();
}

function openAgentBuilder() {
  builderType = 'agent';
  builderMessages = [];
  builderResult = null;
  document.getElementById('builder-title').textContent = 'AI Agent Builder';
  document.getElementById('builder-input').placeholder = 'Describe the agent you want to create...';
  resetBuilderUI();
  document.getElementById('builder-modal').classList.remove('hidden');
  document.getElementById('builder-input').focus();
}

function resetBuilderUI() {
  document.getElementById('builder-messages').innerHTML = '';
  document.getElementById('builder-save-bar').classList.add('hidden');
  document.getElementById('builder-input').value = '';
  document.getElementById('builder-send-btn').disabled = false;
}

function closeBuilder() {
  document.getElementById('builder-modal').classList.add('hidden');
  builderStreaming = false;
}

// "Finish & Save" — sends a finalize message, then auto-saves once we get the definition
async function finishBuilder() {
  if (builderStreaming) return;

  const finishMsg = builderType === 'skill'
    ? 'Please finalize the skill now. Output the complete skill_definition JSON block with all fields (name, description, language, script, parameters) so it can be saved.'
    : 'Please finalize the agent now. Output the complete agent_definition JSON block with all fields (name, description, system_prompt, tools, model) so it can be saved.';

  builderMessages.push({ role: 'user', content: finishMsg });
  addMessage('builder-messages', 'Finishing and saving...', 'user');
  builderStreaming = true;
  document.getElementById('builder-send-btn').disabled = true;
  document.getElementById('builder-finish-btn').disabled = true;

  const area = document.getElementById('builder-messages');
  const loadingEl = document.createElement('div');
  loadingEl.className = 'msg msg-assistant';
  loadingEl.innerHTML = '<span class="spinner"></span> Generating final definition...';
  area.appendChild(loadingEl);
  area.scrollTop = area.scrollHeight;

  const endpoint = builderType === 'skill' ? '/api/build/skill' : '/api/build/agent';

  try {
    const res = await fetch(API + endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages: builderMessages })
    });

    loadingEl.remove();

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Request failed' }));
      addMessage('builder-messages', 'Error: ' + (err.error || res.statusText), 'error');
    } else {
      const data = await res.json();
      const content = data.content || '';
      builderMessages.push({ role: 'assistant', content });
      renderBuilderResponse(area, content);

      const def = extractDefinition(content, builderType);
      if (def) {
        builderResult = def;
        await saveBuilderResult();
      } else {
        addMessage('builder-messages',
          'Could not auto-extract the definition. You can try "Edit first" to save manually.',
          'error');
        document.getElementById('builder-save-bar').classList.remove('hidden');
        document.getElementById('builder-save-label').textContent = 'Auto-save failed — try manually:';
      }
    }
  } catch(e) {
    loadingEl.remove();
    addMessage('builder-messages', 'Error: ' + e.message, 'error');
  }

  builderStreaming = false;
  document.getElementById('builder-send-btn').disabled = false;
  document.getElementById('builder-finish-btn').disabled = false;
  area.scrollTop = area.scrollHeight;
}

function handleBuilderKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendBuilderMessage();
  }
}

async function sendBuilderMessage() {
  const input = document.getElementById('builder-input');
  const message = input.value.trim();
  if (!message || builderStreaming) return;

  builderMessages.push({ role: 'user', content: message });
  addMessage('builder-messages', message, 'user');
  input.value = '';
  autoResize(input);
  builderStreaming = true;
  document.getElementById('builder-send-btn').disabled = true;
  document.getElementById('builder-finish-btn').disabled = true;

  const area = document.getElementById('builder-messages');
  // Show a loading indicator
  const loadingEl = document.createElement('div');
  loadingEl.className = 'msg msg-assistant';
  loadingEl.innerHTML = '<span class="spinner"></span> Thinking...';
  area.appendChild(loadingEl);
  area.scrollTop = area.scrollHeight;

  const endpoint = builderType === 'skill' ? '/api/build/skill' : '/api/build/agent';

  try {
    const res = await fetch(API + endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages: builderMessages })
    });

    loadingEl.remove();

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Request failed' }));
      addMessage('builder-messages', 'Error: ' + (err.error || res.statusText), 'error');
    } else {
      const data = await res.json();
      const content = data.content || '';
      builderMessages.push({ role: 'assistant', content });

      // Render the response with think blocks and code formatting
      renderBuilderResponse(area, content);

      // Check for extractable definition
      const def = extractDefinition(content, builderType);
      if (def) {
        builderResult = def;
        document.getElementById('builder-save-label').textContent =
          'Definition ready — "' + (def.name || 'unnamed') + '"';
        document.getElementById('builder-save-bar').classList.remove('hidden');
      }
    }
  } catch(e) {
    loadingEl.remove();
    addMessage('builder-messages', 'Error: ' + e.message, 'error');
  }

  builderStreaming = false;
  document.getElementById('builder-send-btn').disabled = false;
  document.getElementById('builder-finish-btn').disabled = false;
  area.scrollTop = area.scrollHeight;
}

// Alias for builder — uses shared renderFormattedResponse
function renderBuilderResponse(container, text) {
  renderFormattedResponse(container, text);
}

function extractDefinition(text, type) {
  const marker = type === 'skill' ? 'skill_definition' : 'agent_definition';
  const skillKeys = ['name', 'script', 'language'];
  const agentKeys = ['name', 'system_prompt'];
  const expectedKeys = type === 'skill' ? skillKeys : agentKeys;

  // 1. Try labeled block: ```skill_definition or ```agent_definition
  let match = text.match(new RegExp('```' + marker + '[\\s\\n]+([\\s\\S]*?)```'));
  if (match) {
    const parsed = tryParseJSON(match[1]);
    if (parsed && hasKeys(parsed, expectedKeys)) return parsed;
  }

  // 2. Try ```json block
  match = text.match(/```json[\s\n]+([\s\S]*?)```/);
  if (match) {
    const parsed = tryParseJSON(match[1]);
    if (parsed && hasKeys(parsed, expectedKeys)) return parsed;
  }

  // 3. Try any ``` fenced block that contains JSON
  const fencedBlocks = text.matchAll(/```[a-z_]*[\s\n]+([\s\S]*?)```/g);
  for (const m of fencedBlocks) {
    const parsed = tryParseJSON(m[1]);
    if (parsed && hasKeys(parsed, expectedKeys)) return parsed;
  }

  // 4. Try bare JSON object in the text that has the expected keys
  // Find all { ... } blocks at the top level
  const jsonCandidates = findJSONObjects(text);
  for (const candidate of jsonCandidates) {
    const parsed = tryParseJSON(candidate);
    if (parsed && hasKeys(parsed, expectedKeys)) return parsed;
  }

  return null;
}

function tryParseJSON(str) {
  try {
    return JSON.parse(str.trim());
  } catch(e) {
    return null;
  }
}

function hasKeys(obj, keys) {
  if (!obj || typeof obj !== 'object') return false;
  return keys.every(k => k in obj);
}

// Find JSON object strings in text by matching balanced braces
function findJSONObjects(text) {
  const results = [];
  let i = 0;
  while (i < text.length) {
    if (text[i] === '{') {
      let depth = 0;
      let start = i;
      let inString = false;
      let escaped = false;
      for (let j = i; j < text.length; j++) {
        const c = text[j];
        if (escaped) { escaped = false; continue; }
        if (c === '\\' && inString) { escaped = true; continue; }
        if (c === '"') { inString = !inString; continue; }
        if (inString) continue;
        if (c === '{') depth++;
        else if (c === '}') {
          depth--;
          if (depth === 0) {
            const candidate = text.substring(start, j + 1);
            if (candidate.length > 20) results.push(candidate);
            i = j;
            break;
          }
        }
      }
    }
    i++;
  }
  return results;
}

async function saveBuilderResult() {
  if (!builderResult) return;

  let res;
  if (builderType === 'skill') {
    res = await fetch(API + '/api/skills', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(builderResult)
    });
  } else {
    res = await fetch(API + '/api/agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(builderResult)
    });
  }

  if (res.ok) {
    document.getElementById('builder-save-bar').classList.add('hidden');
    addMessage('builder-messages', 'Saved successfully!', 'assistant');
    // Refresh the list
    if (builderType === 'skill') loadSkills();
    else loadAgentsList();
  } else {
    const err = await res.json();
    addMessage('builder-messages', 'Save failed: ' + (err.error || 'Unknown error'), 'error');
  }
}

async function editBuilderResult() {
  if (!builderResult) return;
  document.getElementById('builder-save-bar').classList.add('hidden');
  closeBuilder();

  if (builderType === 'skill') {
    // Switch to skills page first so the modal is visible
    switchPage('skills');
    document.getElementById('skill-modal-title').textContent = 'Create Skill';
    document.getElementById('skill-save-btn').textContent = 'Create';
    document.getElementById('skill-edit-name').value = '';
    document.getElementById('skill-name').value = builderResult.name || '';
    document.getElementById('skill-name').disabled = false;
    document.getElementById('skill-desc').value = builderResult.description || '';
    document.getElementById('skill-lang').value = builderResult.language || 'bash';
    document.getElementById('skill-script').value = builderResult.script || '';
    try {
      document.getElementById('skill-params').value =
        builderResult.parameters ? JSON.stringify(builderResult.parameters, null, 2) : '';
    } catch(e) {
      document.getElementById('skill-params').value = '';
    }
    document.getElementById('skill-modal').classList.remove('hidden');
  } else {
    // Switch to agents page first so the modal is visible
    switchPage('agents');
    // Wait for showCreateAgent to finish (it loads tools async)
    await showCreateAgent();
    // Now populate with builder result
    document.getElementById('agent-name').value = builderResult.name || '';
    document.getElementById('agent-desc').value = builderResult.description || '';
    document.getElementById('agent-model').value = builderResult.model || '';
    document.getElementById('agent-prompt').value = builderResult.system_prompt || '';
    if (builderResult.tools && builderResult.tools.length > 0) {
      const toolSet = new Set(builderResult.tools);
      document.querySelectorAll('#agent-tools-checklist input[type="checkbox"]').forEach(cb => {
        cb.checked = toolSet.has(cb.value);
      });
    }
  }
}

// --- Self-Improvement Loop ---

let selfImproveRunning = false;
let selfImproveAbort = null; // AbortController for the SSE stream

async function toggleSelfImprove() {
  if (selfImproveRunning) {
    stopSelfImprove();
  } else {
    startSelfImprove();
  }
}

function savePrimeDirective() {
  const val = document.getElementById('si-directive-input').value.trim();
  localStorage.setItem('helm-prime-directive', val);
}

function clearPrimeDirective() {
  document.getElementById('si-directive-input').value = '';
  localStorage.removeItem('helm-prime-directive');
}

function loadPrimeDirective() {
  const saved = localStorage.getItem('helm-prime-directive') || '';
  const input = document.getElementById('si-directive-input');
  if (input) input.value = saved;
  return saved;
}

function saveInterval() {
  const val = parseInt(document.getElementById('si-interval-input').value) || 5;
  localStorage.setItem('helm-evolve-interval', val);
}

function loadInterval() {
  const saved = parseInt(localStorage.getItem('helm-evolve-interval')) || 5;
  const input = document.getElementById('si-interval-input');
  if (input) input.value = saved;
  return saved;
}

async function startSelfImprove() {
  const directive = loadPrimeDirective();
  const interval = loadInterval();
  try {
    const res = await fetch(API + '/api/self-improve/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ interval_minutes: interval, prime_directive: directive })
    });
    if (!res.ok) {
      const err = await res.json();
      alert('Error: ' + (err.error || 'Failed to start'));
      return;
    }
  } catch(e) {
    alert('Error: ' + e.message);
    return;
  }

  selfImproveRunning = true;
  document.getElementById('self-improve-btn').textContent = 'Stop';
  document.getElementById('self-improve-btn').classList.add('running');
  document.getElementById('self-improve-panel').classList.remove('hidden');
  document.getElementById('si-log').innerHTML = '';
  const emptyState = document.getElementById('evolve-empty-state');
  if (emptyState) emptyState.style.display = 'none';
  const indicator = document.getElementById('evolve-status-indicator');
  if (indicator) indicator.classList.add('running');
  loadPrimeDirective();
  loadInterval();

  loadSelfImproveGoals();
  connectSelfImproveStream();
}

async function stopSelfImprove() {
  await fetch(API + '/api/self-improve/stop', { method: 'POST' }).catch(() => {});
  selfImproveRunning = false;
  document.getElementById('self-improve-btn').textContent = 'Start';
  document.getElementById('self-improve-btn').classList.remove('running');
  const indicator = document.getElementById('evolve-status-indicator');
  if (indicator) indicator.classList.remove('running');
  if (selfImproveAbort) {
    selfImproveAbort.abort();
    selfImproveAbort = null;
  }
}

function connectSelfImproveStream() {
  selfImproveAbort = new AbortController();
  const log = document.getElementById('si-log');

  fetch(API + '/api/self-improve/stream', { signal: selfImproveAbort.signal })
    .then(response => {
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let currentEvent = '';
      let dataLines = [];
      let doneFired = false;

      function processEvent(event, data) {
        if (event === 'thinking') {
          try {
            const info = JSON.parse(data);
            const tag = info.agent_name ? agentTag(info.agent_name) : '';
            const el = document.createElement('div');
            el.className = 'msg msg-thinking';
            el.innerHTML = tag + escapeHtml(truncate(info.content || data, 300));
            log.appendChild(el);
          } catch(e) {
            addMessageTo(log, data, 'thinking');
          }
        } else if (event === 'tool_call') {
          try {
            const tc = JSON.parse(data);
            const tag = tc.agent_name ? agentTag(tc.agent_name) : '';
            const el = document.createElement('div');
            el.className = 'msg msg-tool';
            el.innerHTML = tag + '<div class="tool-name">' + escapeHtml(tc.name) + '</div>' +
              '<div>' + escapeHtml(truncate(tc.arguments, 200)) + '</div>';
            log.appendChild(el);
          } catch(e) {}
        } else if (event === 'tool_result') {
          try {
            const info = JSON.parse(data);
            const el = document.createElement('div');
            el.className = 'msg msg-tool';
            el.innerHTML = '<div class="tool-result">' + escapeHtml(truncate(info.content || data, 300)) + '</div>';
            log.appendChild(el);
          } catch(e) {}
        } else if (event === 'answer') {
          try {
            const info = JSON.parse(data);
            renderFormattedResponse(log, info.content || data);
          } catch(e) {
            renderFormattedResponse(log, data);
          }
        } else if (event === 'cycle_end') {
          const el = document.createElement('div');
          el.className = 'msg msg-resume-indicator';
          el.textContent = '--- Cycle complete. Next cycle in 5 minutes. ---';
          log.appendChild(el);
          loadSelfImproveGoals();
          // Update cycle badge
          fetch(API + '/api/self-improve/status').then(r => r.json()).then(s => {
            document.getElementById('si-cycle-badge').textContent = 'Cycle ' + s.cycle;
          }).catch(() => {});
        } else if (event === 'error') {
          addMessageTo(log, 'Error: ' + data, 'error');
        } else if (event === 'sub_agent_start') {
          try {
            const info = JSON.parse(data);
            const el = document.createElement('div');
            el.className = 'msg msg-sub-agent-start';
            el.innerHTML = '<strong>' + escapeHtml(info.agent_name) + '</strong> started: ' + escapeHtml(truncate(info.task, 100));
            log.appendChild(el);
          } catch(e) {}
        } else if (event === 'sub_agent_done') {
          try {
            const info = JSON.parse(data);
            const el = document.createElement('div');
            el.className = 'msg msg-sub-agent-done';
            el.innerHTML = '<strong>' + escapeHtml(info.agent_name) + '</strong> ' + escapeHtml(info.status);
            log.appendChild(el);
          } catch(e) {}
        } else if (event === 'escalation') {
          try {
            const info = JSON.parse(data);
            const el = document.createElement('div');
            el.className = 'msg msg-escalation';
            const tag = info.agent_name ? agentTag(info.agent_name) : '';
            el.innerHTML = tag +
              '<div class="escalation-question">' + escapeHtml(info.question) + '</div>' +
              '<div class="escalation-input">' +
              '<input type="text" class="escalation-field" placeholder="Type your response..." ' +
              'onkeydown="if(event.key===\'Enter\')respondToEscalation(this)">' +
              '<button class="action-btn" onclick="respondToEscalation(this.previousElementSibling)">Respond</button>' +
              '</div>';
            log.appendChild(el);
          } catch(e) {}
        }
        log.scrollTop = log.scrollHeight;
      }

      function read() {
        reader.read().then(({ done, value }) => {
          if (done) return;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop();
          for (const line of lines) {
            if (line.startsWith('event: ')) {
              currentEvent = line.slice(7);
              dataLines = [];
            } else if (line.startsWith('data: ')) {
              dataLines.push(line.slice(6));
            } else if (line === '' && currentEvent) {
              processEvent(currentEvent, dataLines.join('\n'));
              currentEvent = '';
              dataLines = [];
            }
          }
          read();
        }).catch(() => {});
      }
      read();
    }).catch(() => {});
}

async function loadSelfImproveGoals() {
  try {
    const res = await fetch(API + '/api/goals');
    const goals = await res.json();
    const container = document.getElementById('si-goals');
    if (goals.length === 0) {
      container.innerHTML = '<span style="font-size:11px;color:var(--text-dimmer)">No goals yet — agent will create them</span>';
      return;
    }
    container.innerHTML = goals.map(g => {
      const cls = g.status === 'completed' ? 'completed' : g.status === 'paused' ? 'paused' : 'active';
      return '<span class="si-goal-chip ' + cls + '" title="' + escapeHtml(g.description || '') + '">' +
        escapeHtml(g.title) + '</span>';
    }).join('');
  } catch(e) {}
}

// Check if self-improve is already running on page load
async function checkSelfImproveStatus() {
  try {
    const res = await fetch(API + '/api/self-improve/status');
    const status = await res.json();
    if (status.running) {
      selfImproveRunning = true;
      document.getElementById('self-improve-btn').textContent = 'Stop';
      document.getElementById('self-improve-btn').classList.add('running');
      document.getElementById('self-improve-panel').classList.remove('hidden');
      document.getElementById('si-cycle-badge').textContent = 'Cycle ' + status.cycle;
      const emptyState = document.getElementById('evolve-empty-state');
      if (emptyState) emptyState.style.display = 'none';
      const indicator = document.getElementById('evolve-status-indicator');
      if (indicator) indicator.classList.add('running');
      loadPrimeDirective();
      loadInterval();
      loadSelfImproveGoals();
      connectSelfImproveStream();
    }
  } catch(e) {}
}

// --- Themes ---

const THEME_ICONS = {
  '':            { logo: '✦', chat: '📡', agent: '🖖', agents: '🛸', skills: '⚡', evolution: '🔄', sessions: '📋', settings: '⚙️' },
  'matrix':      { logo: '⌬', chat: '▶',  agent: '◉',  agents: '◈',  skills: '⚙',  evolution: '⟳',  sessions: '▤',  settings: '⌘'  },
  'netrunner':   { logo: '◇', chat: '⟐',  agent: '⬡',  agents: '⬢',  skills: '⚡', evolution: '⥁',  sessions: '▥',  settings: '⚙'  },
  'snowcrash':   { logo: '☣', chat: '⌁',  agent: '⍟',  agents: '⎊',  skills: '⚒',  evolution: '⟳',  sessions: '⏣',  settings: '⚙'  },
  'neuromancer': { logo: '◈', chat: '⌖',  agent: '⍾',  agents: '⎈',  skills: '⚛',  evolution: '⥁',  sessions: '☰',  settings: '⚙'  },
  'bladerunner': { logo: '▲', chat: '◌',  agent: '◎',  agents: '⊛',  skills: '⚡', evolution: '⟳',  sessions: '≡',  settings: '⚙'  },
  'lcars':       { logo: '◆', chat: '▸',  agent: '●',  agents: '◐',  skills: '◇',  evolution: '◑',  sessions: '▪',  settings: '■'  },
};

const THEMES = [
  { id: '',            name: 'Default',      colors: ['#0d1117', '#7c3aed', '#58a6ff', '#3fb950'] },
  { id: 'matrix',      name: 'Matrix',       colors: ['#0a0a0a', '#00ff41', '#00ccff', '#22aa22'] },
  { id: 'netrunner',   name: 'Netrunner',    colors: ['#0a0a12', '#ff00ff', '#00ccff', '#00ffcc'] },
  { id: 'snowcrash',   name: 'Snow Crash',   colors: ['#000000', '#ff6600', '#ffcc00', '#3399ff'] },
  { id: 'neuromancer', name: 'Neuromancer',  colors: ['#050a18', '#ffaa00', '#4488ff', '#00ddaa'] },
  { id: 'bladerunner', name: 'Blade Runner', colors: ['#0a0808', '#ff8844', '#44aacc', '#44ddaa'] },
  { id: 'lcars',       name: 'LCARS',        colors: ['#000000', '#ff9900', '#cc99ff', '#9999ff'] },
];

function renderThemePicker() {
  const picker = document.getElementById('theme-picker');
  if (!picker) return;
  const current = document.documentElement.getAttribute('data-theme') || '';

  picker.innerHTML = THEMES.map(t => {
    const active = t.id === current ? ' active' : '';
    const swatches = t.colors.map(c =>
      '<span style="background:' + c + '"></span>').join('');
    return '<div class="theme-card' + active + '" onclick="setTheme(\'' + t.id + '\')">' +
      '<div class="theme-card-preview">' + swatches + '</div>' +
      '<div class="theme-card-name">' + escapeHtml(t.name) + '</div></div>';
  }).join('');
}

function setTheme(themeId) {
  if (themeId) {
    document.documentElement.setAttribute('data-theme', themeId);
  } else {
    document.documentElement.removeAttribute('data-theme');
  }
  localStorage.setItem('helm-theme', themeId);
  applyThemeIcons(themeId);
  renderThemePicker();
}

function setFontSize(px) {
  document.documentElement.style.setProperty('--font-size', px + 'px');
  localStorage.setItem('helm-font-size', px);
  const label = document.getElementById('font-size-label');
  if (label) label.textContent = px + 'px';
}

function loadSavedFontSize() {
  const saved = localStorage.getItem('helm-font-size');
  if (saved) {
    document.documentElement.style.setProperty('--font-size', saved + 'px');
    const slider = document.getElementById('font-size-slider');
    if (slider) slider.value = saved;
    const label = document.getElementById('font-size-label');
    if (label) label.textContent = saved + 'px';
  }
}

function loadSavedTheme() {
  const saved = localStorage.getItem('helm-theme') || '';
  if (saved) {
    document.documentElement.setAttribute('data-theme', saved);
  }
  applyThemeIcons(saved);
}

function applyThemeIcons(themeId) {
  const icons = THEME_ICONS[themeId] || THEME_ICONS[''];
  document.querySelectorAll('[data-icon]').forEach(el => {
    const key = el.getAttribute('data-icon');
    if (icons[key]) {
      el.textContent = icons[key];
    }
  });
}

// --- Init ---

loadSavedTheme();
loadSavedFontSize();
loadPrimeDirective();
loadInterval();
loadMemoryStats();
loadConfig().catch(() => {});
checkSelfImproveStatus();
// Hide flow tab until agent page is active
const _flowTab = document.getElementById('deleg-toggle');
if (_flowTab) _flowTab.style.display = 'none';
