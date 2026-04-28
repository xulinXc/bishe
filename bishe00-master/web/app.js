const api = (path, body) => fetch(path, { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body) }).then(async r=>{ const text = await r.text(); let data = {}; try{ data = JSON.parse(text); }catch{ /* non-JSON */ } if(!r.ok){ const msg = (data && (data.error||data.message)) || text || '请求失败'; throw new Error(msg); } return data; })

// Directory picker helper (best-effort)
function pickFiles(accept, multiple=true){
  return new Promise((resolve)=>{
    const inp = document.createElement('input')
    inp.type = 'file'
    if (multiple) inp.multiple = true
    if (accept) inp.accept = accept
    inp.onchange = ()=>{ resolve(Array.from(inp.files||[])) }
    inp.click()
  })
}
const sse = (taskId, onMsg) => {
  const es = new EventSource(`/events?task=${taskId}`)
  es.onmessage = (e)=>{ try{ const msg = JSON.parse(e.data); onMsg(msg) }catch{} }
  es.onerror = (e)=>{ console.warn('SSE error', e); try{ es.close() }catch{} try{ notify('warn','SSE连接异常，任务可能已结束或网络中断') }catch{} }
  return es
}

function setProgress(prefix, percent, text){
  const bar = document.getElementById(`${prefix}-bar`); if(bar) bar.style.width = `${percent}%`
  const txt = document.getElementById(`${prefix}-text`); if(txt) txt.textContent = text
}
// lightweight toast notifications
function notify(type, text){
  try{
    let bar = document.getElementById('toast-bar');
    if(!bar){
      bar = document.createElement('div');
      bar.id='toast-bar';
      bar.style.cssText='position:fixed;top:10px;right:10px;z-index:9999;display:flex;flex-direction:column;gap:6px';
      document.body.appendChild(bar);
    }
    const item = document.createElement('div');
    const bg = type==='error'?'#b91c1c':(type==='warn'?'#b45309':'#2563eb');
    item.style.cssText=`padding:8px 12px;border-radius:8px;color:#fff;background:${bg};box-shadow:0 2px 8px rgba(0,0,0,.3)`;
    item.textContent = String(text||'');
    bar.appendChild(item);
    setTimeout(()=>{ try{ bar.removeChild(item) }catch{} }, 4000);
  }catch{}
}
// cap list items and provide expand/collapse control
function capList(outEl, limit){
  try{
    if(!outEl) return;
    const id = outEl.id || '';
    if(outEl.dataset.expanded==='1') return;
    const children = Array.from(outEl.children||[]);
    const moreId = id? `${id}-more` : '';
    const tooMany = children.length > limit;
    // hide older ones
    children.slice(limit).forEach(el=>{ el.style.display='none' });
    // control button
    if(moreId){
      let ctrl = document.getElementById(moreId);
      if(!ctrl){
        ctrl = document.createElement('div');
        ctrl.id = moreId;
        ctrl.style.cssText='margin:6px 0;';
        const btn = document.createElement('button');
        btn.className='btn btn-ghost';
        btn.textContent='显示全部';
        btn.onclick=()=>{
          const expanded = outEl.dataset.expanded==='1';
          if(expanded){
            outEl.dataset.expanded='0';
            Array.from(outEl.children||[]).slice(limit).forEach(el=>{ el.style.display='none' });
            btn.textContent='显示全部';
          }else{
            outEl.dataset.expanded='1';
            Array.from(outEl.children||[]).forEach(el=>{ el.style.display='' });
            btn.textContent='折叠';
          }
        };
        ctrl.appendChild(btn);
        if(outEl.parentNode){ outEl.parentNode.insertBefore(ctrl, outEl.nextSibling); }
      }
      ctrl.style.display = tooMany? '' : 'none';
    }
  }catch(e){}
}
function createRow(id, cls, leftHTML, rightHTML){
  const out = document.getElementById(id); if(!out) return;
  const row = document.createElement('div'); row.className = `row ${cls||''}`
  const left = document.createElement('div'); left.className = 'left'; left.innerHTML = `<span class="icon"></span>${leftHTML||''}`
  const right = document.createElement('div'); right.className = 'right'; right.innerHTML = rightHTML||''
  row.appendChild(left); row.appendChild(right); out.prepend(row)
  // limit list size for better UX on heavy tasks
  capList(out, 300)
}
function escapeHtml(text){
  if(text===undefined || text===null) return ''
  const div = document.createElement('div')
  div.textContent = String(text)
  return div.innerHTML
}
function downloadText(filename, text, mime){
  const blob = new Blob([text||''], { type: (mime||'text/plain') + ';charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename || `neonscan-${Date.now()}.txt`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
const pendingBindings = {}
window.pendingBindings = window.pendingBindings || pendingBindings
window.__bindings = window.__bindings || {}
const bind = function(id, fn) {
  pendingBindings[id] = fn
  window.__bindings[id] = fn
  var el = window.document ? window.document.getElementById(id) : null
  if(el) {
    el.onclick = fn
    console.log('Button ' + id + ' bound successfully')
  } else {
    console.warn('Button ' + id + ' not found, saved for later binding')
  }
}

// number input adjustment
function adjustNumber(inputId, delta) {
  const input = document.getElementById(inputId)
  if (!input) return
  const current = parseInt(input.value) || 0
  const newValue = Math.max(0, current + delta)
  input.value = newValue
}

// Make adjustNumber globally available
window.adjustNumber = adjustNumber

// file selection display control
function showFileArea(areaId) {
  const area = document.getElementById(areaId)
  if (area) area.style.display = 'block'
}

function hideFileArea(areaId) {
  const area = document.getElementById(areaId)
  if (area) area.style.display = 'none'
}

// selections and rendering
function renderSelected(containerId, inputId, areaId){
  const c = document.getElementById(containerId); if(!c) return;
  const val = (document.getElementById(inputId)?.value||'').trim()
  const arr = val? val.split(';').filter(Boolean): []
  c.innerHTML = ''
  
  // show/hide file area based on whether files are selected
  if (areaId) {
    if (arr.length > 0) {
      showFileArea(areaId)
    } else {
      hideFileArea(areaId)
    }
  }
  
  arr.forEach((p,idx)=>{
    const pill = document.createElement('span'); pill.className='pill'
    pill.innerHTML = `${p} <button data-index="${idx}">×</button>`
    pill.querySelector('button').onclick = ()=>{
      const nv = arr.filter((_,i)=>i!==idx).join(';')
      document.getElementById(inputId).value = nv
      renderSelected(containerId, inputId, areaId)
    }
    c.appendChild(pill)
  })
}

// bind pickers
bind('ds-pick', async ()=>{ 
  const files = await pickFiles('.txt,.lst,.dic')
  if(!files || !files.length) return;
  const form = new FormData(); files.forEach(f=>form.append('files', f))
  const res = await fetch('/upload',{method:'POST', body:form}).then(r=>r.json())
  const list = res.paths||[]; const inp = document.getElementById('ds-dict');
  const cur = inp.value? inp.value.split(';').filter(Boolean):[]
  inp.value = cur.concat(list).join(';')
  renderSelected('ds-dict-list','ds-dict','ds-file-area')
})

bind('pc-pick', async ()=>{ 
  const files = await pickFiles('.json,.yaml,.yml')
  if(!files || !files.length) return;
  const form = new FormData(); files.forEach(f=>form.append('files', f))
  const res = await fetch('/upload',{method:'POST', body:form}).then(r=>r.json())
  const list = res.paths||[]; const inp = document.getElementById('pc-dir');
  const cur = inp.value? inp.value.split(';').filter(Boolean):[]
  inp.value = cur.concat(list).join(';')
  renderSelected('pc-dir-list','pc-dir','pc-file-area')
})

bind('ex-pick', async ()=>{ 
  const files = await pickFiles('.json,.yaml,.yml')
  if(!files || !files.length) return;
  const form = new FormData(); files.forEach(f=>form.append('files', f))
  const res = await fetch('/upload',{method:'POST', body:form}).then(r=>r.json())
  const list = res.paths||[]; const inp = document.getElementById('ex-dir');
  const cur = inp.value? inp.value.split(';').filter(Boolean):[]
  inp.value = cur.concat(list).join(';')
  renderSelected('ex-dir-list','ex-dir','ex-file-area')
})

// --- form state persistence (A) ---
const PERSIST_KEYS = [
  'ps-host','ps-ports','ps-conc','ps-timeout','ps-tcp','ps-udp','ps-banner',
  'ds-base','ds-dict','ds-conc','ds-timeout','ds-headers','ds-inc-any','ds-inc-all','ds-exc-any','ds-minlen',
  'pc-base','pc-dir','pc-conc','pc-timeout',
  'ex-base','ex-dir','ex-conc','ex-timeout','ex-ai-provider','ex-ai-baseurl','ex-ai-model',
  'wb-urls','wb-conc','wb-timeout','wb-headers','wb-follow','wb-favicon','wb-robots',
  'wf-base','wf-path','wf-methods','wf-strategies','wf-match','wf-payloads','wf-conc','wf-timeout'
]

function saveFormState(){
  try{
    const s = {}
    PERSIST_KEYS.forEach(k=>{
      const el = document.getElementById(k)
      if(!el) return
      if(el.type==='checkbox') s[k] = el.checked
      else s[k] = el.value
    })
    localStorage.setItem('neonscan-forms', JSON.stringify(s))
  }catch(e){console.warn('saveFormState failed', e)}
}
function restoreFormState(){
  try{
    const raw = localStorage.getItem('neonscan-forms')
    if(!raw) return
    const s = JSON.parse(raw)
    PERSIST_KEYS.forEach(k=>{
      const el = document.getElementById(k)
      if(!el || s[k]===undefined) return
      if(el.type==='checkbox') el.checked = !!s[k]
      else el.value = s[k]
    })
    // re-render selected lists
    renderSelected('ds-dict-list','ds-dict','ds-file-area')
    renderSelected('pc-dir-list','pc-dir','pc-file-area')
    renderSelected('ex-dir-list','ex-dir','ex-file-area')
  }catch(e){console.warn('restoreFormState failed', e)}
}
// auto-save on change
function attachPersistOnChange(){
  PERSIST_KEYS.forEach(k=>{
    const el = document.getElementById(k)
    if(!el) return
    el.addEventListener('change', saveFormState)
    el.addEventListener('input', saveFormState)
  })
}
// restore immediately and attach listeners
restoreFormState()
attachPersistOnChange()

// ensure late bindings happen after DOM ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', bindAllButtons)
} else {
  bindAllButtons()
}

// maintain running tasks and stop actions
const running = {}
function persistRunningTasks(){
  const simple = {}
  Object.keys(running).forEach(k=>{ if(running[k] && running[k].id){ simple[k]=running[k].id } })
  localStorage.setItem('neonscan-running', JSON.stringify(simple))
}
function startTask(key, taskId, onMsg){
  // close any existing EventSource for this key
  if(running[key]?.es) try{ running[key].es.close() }catch{}
  const es = sse(taskId, (m)=>{
    if(m.type==='end'){ delete running[key]; persistRunningTasks() }
    onMsg(m)
  })
  running[key] = { id: taskId, es }
  persistRunningTasks()
}
async function stopTask(key, prefix){
  const t = running[key]; if(!t) return;
  try{ await fetch(`/task/stop?task=${encodeURIComponent(t.id)}`, {method:'POST'}) }catch{}
  try{ t.es.close() }catch{}
  delete running[key]
  persistRunningTasks()
  setProgress(prefix, 100, '已停止')
}
bind('ps-stop', ()=>{
  stopTask('ps','ps')
  stopTask('ps-tcp','ps')
  stopTask('ps-udp','ps')
})
bind('ps-clear', ()=>{ const el=document.getElementById('ps-out'); if(el) el.innerHTML=''; setProgress('ps',0,'0%') })
bind('ds-stop', ()=>stopTask('ds','ds'))
bind('ds-clear', ()=>{ const el=document.getElementById('ds-out'); if(el) el.innerHTML=''; setProgress('ds',0,'0%') })
bind('pc-stop', ()=>stopTask('pc','pc'))
bind('pc-clear', ()=>{ const el=document.getElementById('pc-out'); if(el) el.innerHTML=''; setProgress('pc',0,'0%') })
bind('ex-stop', ()=>stopTask('ex','ex'))
bind('ex-clear', ()=>{ const el=document.getElementById('ex-out'); if(el) el.innerHTML=''; setProgress('ex',0,'0%'); try{ setPendingExps([]); }catch{} try{ renderPendingExps() }catch{} })
bind('wb-stop', ()=>stopTask('wb','wb'))
bind('wb-clear', ()=>{ const el=document.getElementById('wb-out'); if(el) el.innerHTML=''; setProgress('wb',0,'0%') })
bind('wf-stop', ()=>stopTask('wf','wf'))
bind('wf-clear', ()=>{ const el=document.getElementById('wf-out'); if(el) el.innerHTML=''; setProgress('wf',0,'0%') })

// 端口扫描
async function handlePsStart(){
  const host = document.getElementById('ps-host')?.value.trim()
  const ports = document.getElementById('ps-ports')?.value.trim()
  const concurrency = parseInt(document.getElementById('ps-conc')?.value)||500
  const timeoutMs = parseInt(document.getElementById('ps-timeout')?.value)||300
  const tcpChecked = !!document.getElementById('ps-tcp')?.checked
  const udpChecked = !!document.getElementById('ps-udp')?.checked
  const grabBanner = !!document.getElementById('ps-banner')?.checked
  
  if (!tcpChecked && !udpChecked) {
    alert('请至少选择一种扫描类型')
    return
  }
  
  if (tcpChecked && udpChecked) {
    const {taskId: tcpTaskId} = await api('/scan/ports',{host,ports,concurrency,timeoutMs,scanType:'tcp',grabBanner})
    startTask('ps-tcp', tcpTaskId, (m)=>{
      if(m.type==='start'||m.type==='progress'){ setProgress('ps', m.percent, `TCP: ${m.progress}`) }
      if(m.type==='find'){ const proto = (m.data.proto||'tcp').toUpperCase(); const banner = (m.data.banner||'').replace(/\s+/g,' ').slice(0,200); const right = `<span class="badge">${proto}</span>` + (banner?` <span class="badge" title="${banner}">banner</span>`:''); createRow('ps-out','success', `<strong>TCP OPEN</strong> : <span class="badge green">${m.data.port}</span>`, right) }
      if(m.type==='end'){ setProgress('ps', 100, `TCP: ${m.progress}`); createRow('ps-out','info', 'TCP扫描完成', `<span>${m.progress}</span>`) }
    })
    const {taskId: udpTaskId} = await api('/scan/ports',{host,ports,concurrency,timeoutMs,scanType:'udp',grabBanner:false})
    startTask('ps-udp', udpTaskId, (m)=>{
      if(m.type==='start'||m.type==='progress'){ setProgress('ps', m.percent, `UDP: ${m.progress}`) }
      if(m.type==='find'){ const proto = (m.data.proto||'udp').toUpperCase(); const banner = (m.data.banner||'').replace(/\s+/g,' ').slice(0,200); const right = `<span class="badge">${proto}</span>` + (banner?` <span class="badge" title="${banner}">banner</span>`:''); createRow('ps-out','success', `<strong>UDP OPEN</strong> : <span class="badge green">${m.data.port}</span>`, right) }
      if(m.type==='end'){ setProgress('ps', 100, `UDP: ${m.progress}`); createRow('ps-out','info', 'UDP扫描完成', `<span>${m.progress}</span>`) }
    })
  } else {
    const scanType = tcpChecked ? 'tcp' : 'udp'
    const {taskId} = await api('/scan/ports',{host,ports,concurrency,timeoutMs,scanType,grabBanner})
    startTask('ps', taskId, (m)=>{
      if(m.type==='start'||m.type==='progress'){ setProgress('ps', m.percent, m.progress) }
      if(m.type==='find'){ 
        const proto = (m.data.proto||'tcp').toUpperCase(); 
        const banner = (m.data.banner||'').replace(/\s+/g,' ').slice(0,200); 
        const right = `<span class="badge">${proto}</span>` + (banner?` <span class="badge" title="${banner}">banner</span>`:''); 
        createRow('ps-out','success', `<strong>OPEN</strong> : <span class="badge green">${m.data.port}</span>`, right);
        addPortScanResult({
          target: host,
          scanType: scanType,
          result: m.data
        });
      }
      if(m.type==='end'){ setProgress('ps', 100, m.progress); createRow('ps-out','info', '扫描完成', `<span>${m.progress}</span>`) }
    })
  }
}
bind('ps-start', handlePsStart)

// 加载内置字典列表
async function loadBuiltinDicts() {
  try {
    const response = await fetch('/api/dicts')
    const dicts = await response.json()
    const container = document.getElementById('ds-builtin-dicts')
    if (!container) return
    
    container.innerHTML = ''
    
    // 按分类顺序显示
    const categoryOrder = ['通用', 'PHP', 'Java', 'ASP', 'Python', 'Ruby', 'Node.js', 'Go', '其他']
    
    for (const category of categoryOrder) {
      if (!dicts[category] || dicts[category].length === 0) continue
      
      const categoryDiv = document.createElement('div')
      categoryDiv.className = 'dict-category'
      
      const titleDiv = document.createElement('div')
      titleDiv.className = 'dict-category-title'
      titleDiv.textContent = category
      categoryDiv.appendChild(titleDiv)
      
      const itemsDiv = document.createElement('div')
      itemsDiv.className = 'dict-items'
      
      for (const dict of dicts[category]) {
        const itemDiv = document.createElement('div')
        itemDiv.className = 'dict-item'
        
        const checkbox = document.createElement('input')
        checkbox.type = 'checkbox'
        checkbox.id = `dict-${dict.name}`
        checkbox.value = dict.name
        checkbox.dataset.category = category
        
        const label = document.createElement('label')
        label.htmlFor = `dict-${dict.name}`
        label.textContent = dict.name
        
        itemDiv.appendChild(checkbox)
        itemDiv.appendChild(label)
        itemsDiv.appendChild(itemDiv)
      }
      
      categoryDiv.appendChild(itemsDiv)
      container.appendChild(categoryDiv)
    }
    
    // 如果没有字典，显示提示
    if (container.children.length === 0) {
      container.innerHTML = '<div style="text-align: center; color: var(--muted); padding: 20px;">未找到内置字典文件</div>'
    }
  } catch (err) {
    console.error('加载内置字典失败:', err)
    const container = document.getElementById('ds-builtin-dicts')
    if (container) {
      container.innerHTML = '<div style="text-align: center; color: #ef4444; padding: 20px;">加载字典列表失败: ' + (err.message || err) + '</div>'
    }
  }
}

// 页面加载时加载字典列表
if (document.getElementById('ds-builtin-dicts')) {
  loadBuiltinDicts()
}

// 字典文件选择
bind('ds-pick', ()=>{
  const input = document.createElement('input')
  input.type = 'file'
  input.multiple = true
  input.accept = '.txt,.dic,.dict'
  input.onchange = async (e) => {
    const files = e.target.files
    if (!files || files.length === 0) return
    
    const formData = new FormData()
    for (let i = 0; i < files.length; i++) {
      formData.append('files', files[i])
    }
    
    try {
      const btn = document.getElementById('ds-pick')
      const originalText = btn.textContent
      btn.textContent = '上传中...'
      btn.disabled = true
      
      const res = await fetch('/upload', {
        method: 'POST',
        body: formData
      })
      
      if (!res.ok) throw new Error('Upload failed')
      
      const data = await res.json()
      if (data.paths && data.paths.length > 0) {
        const inputEl = document.getElementById('ds-dict')
        const current = inputEl.value.trim()
        const newPaths = data.paths.join(';')
        inputEl.value = current ? current + ';' + newPaths : newPaths
        notify('success', `已上传 ${data.paths.length} 个字典文件`)
      }
      
      btn.textContent = originalText
      btn.disabled = false
    } catch (err) {
      console.error(err)
      notify('error', '文件上传失败')
      document.getElementById('ds-pick').textContent = '选择'
      document.getElementById('ds-pick').disabled = false
    }
  }
  input.click()
})

// 目录扫描
bind('ds-start', async ()=>{
  try{
    const baseUrl = document.getElementById('ds-base')?.value.trim()
    const dictValue = document.getElementById('ds-dict')?.value.trim()
    const dictPaths = dictValue? dictValue.split(';').filter(Boolean): []
    
    // 获取选中的内置字典
    const builtinDicts = []
    const checkboxes = document.querySelectorAll('#ds-builtin-dicts input[type="checkbox"]:checked')
    checkboxes.forEach(cb => {
      builtinDicts.push(cb.value)
    })
    
    if(!baseUrl){ notify('error','请填写基础URL'); return }
    if(!dictPaths.length && !builtinDicts.length){ notify('error','请至少选择一个内置字典或自定义字典文件'); return }
    const concurrency = parseInt(document.getElementById('ds-conc')?.value)||200
    const timeoutMs = parseInt(document.getElementById('ds-timeout')?.value)||1500
    const {taskId} = await api('/scan/dirs',{baseUrl,dictPaths,builtinDicts,concurrency,timeoutMs})
    startTask('ds', taskId, (m)=>{
      if(m.type==='start'||m.type==='progress'){ setProgress('ds', m.percent, m.progress) }
      if(m.type==='find'){
        const status = m.data.status;
        const color = status===200?'green':(status===403?'yellow':'blue')
        const extra = []
        if(typeof m.data.length==='number') extra.push(`<span class="badge">len: ${m.data.length}</span>`) 
        if(m.data.location) extra.push(`<span class="badge">Location: ${m.data.location}</span>`) 
        createRow('ds-out','info', `${status} <span class="badge ${color}">${m.data.url}</span>`, extra.join(' '))
      }
      if(m.type==='end'){ 
        setProgress('ds', 100, m.progress); 
        createRow('ds-out','info', '扫描完成', `<span>${m.progress}</span>`)
        const outEl = document.getElementById('ds-out');
        if(outEl && outEl.children.length===0){ notify('warn','未发现结果，部分请求可能无效或被跳过'); }
      }
    })
  }catch(err){
    console.warn('目录扫描启动失败', err)
    notify('error','目录扫描启动失败：' + (err?.message||err))
  }
})

// POC 扫描
bind('pc-start', async ()=>{
  const baseUrl = document.getElementById('pc-base')?.value.trim()
  const pocSource = document.querySelector('input[name="poc-source"]:checked')?.value || 'builtin'
  let pocDir = ''
  let pocPaths = []
  
  if (pocSource === 'builtin') {
    // 使用内置POC，不传pocDir，后端会自动使用内置目录
    pocDir = ''
  } else {
    // 使用自定义POC目录
    const pocValue = document.getElementById('pc-dir')?.value.trim()
    pocPaths = pocValue? pocValue.split(';').filter(Boolean): []
    // If a single entry and not a file extension, treat as directory for server-side recursive load
    if (pocPaths.length === 1 && !/\.(json|ya?ml)$/i.test(pocPaths[0])) {
      pocDir = pocPaths[0]
      pocPaths = []
    }
  }
  
  const concurrency = parseInt(document.getElementById('pc-conc')?.value)||50
  const timeoutMs = parseInt(document.getElementById('pc-timeout')?.value)||3000
  const {taskId} = await api('/scan/poc',{baseUrl,pocDir,pocPaths,concurrency,timeoutMs})
  startTask('pc', taskId, (m)=>{
    if(m.type==='start'||m.type==='progress'){ 
      let text = m.progress
      if (m.data && m.data.current) {
        text += ` (正在扫描: ${m.data.current})`
      }
      setProgress('pc', m.percent, text) 
    }
    if(m.type==='scan_log'){
      // 显示未发现的结果，使用灰色/绿色
      // 限制显示数量，只显示最近的5条，避免刷屏？不，用户要求显示。
      // 我们可以使用 createRow，它会自动限制数量。
      createRow('pc-out', 'info', `<span>${m.data.poc}</span>`, `<span class="badge" style="background:rgba(16, 185, 129, 0.15); border-color:rgba(16, 185, 129, 0.6); color:#10b981">未发现漏洞</span>`)
    }
    if(m.type==='find'){
        const info = m.data.info||{}
        const detail = [
          info.severity?`<span class="badge">${info.severity}</span>`:'',
          info.name?`<span>${info.name}</span>`:'',
          Array.isArray(info.reference)? info.reference.map(r=>`<a class="badge" href="${r}" target="_blank">ref</a>`).join(' ') : ''
        ].filter(Boolean).join(' ')
        // Payload 中的双引号需要被转义，否则在 data-payload 属性中会截断 JSON
        const payload = encodeURIComponent(JSON.stringify({data:m.data,baseUrl}))
        const btn = `<button class="btn btn-ghost gen-exp-btn" data-payload="${payload}" style="white-space: nowrap;">生成EXP并验证</button>`
        const urlDisplay = m.data.url ? `<span class="badge" style="word-break: break-word; overflow-wrap: break-word; max-width: 600px; display: inline-block; box-sizing: border-box;">${m.data.url}</span>` : ''
        const expDisplay = m.data.exp ? `<span class="code" title="${m.data.exp.replace(/"/g, '&quot;')}">${m.data.exp}</span>` : ''
      
      // 根据severity选择不同的row class
      const severity = (info.severity || '').toLowerCase()
      let rowClass = 'warn' // 默认
      if(severity === 'critical' || severity === 'high') {
        rowClass = 'error'
      } else if(severity === 'medium') {
        rowClass = 'warn'
      } else if(severity === 'low' || severity === 'info' || severity === 'informational') {
        rowClass = 'info'
      }
      
      // 使用 createRow 并在创建后将元素移动到顶部
      createRow('pc-out', rowClass, `<strong>${m.data.poc}</strong> ${urlDisplay}`, `${detail} ${expDisplay} ${btn}`)
      // 将新创建的行移动到容器顶部
      const outContainer = document.getElementById('pc-out')
      if (outContainer && outContainer.lastElementChild) {
        outContainer.prepend(outContainer.lastElementChild)
      }

      // Add to report
      addPocScanResult({
        target: baseUrl,
        result: m.data
      });
    }
    if(m.type==='end'){ 
      setProgress('pc', 100, m.progress); 
      createRow('pc-out','success', '扫描完成', `<span>${m.progress}</span>`);
      
      // 扫描完成后，将所有高危/中危漏洞移动到最前面
      const outContainer = document.getElementById('pc-out');
      if(outContainer) {
        const rows = Array.from(outContainer.children);
        // 筛选出发现漏洞的行 (class 包含 error, warn, info 且不是扫描完成的提示)
        const vulnRows = rows.filter(row => {
          return (row.classList.contains('error') || row.classList.contains('warn') || row.classList.contains('info')) && 
                 !row.textContent.includes('扫描完成') && 
                 !row.textContent.includes('未发现漏洞');
        });
        
        // 按严重程度排序：error > warn > info
        vulnRows.sort((a, b) => {
          const score = (row) => {
            if(row.classList.contains('error')) return 3;
            if(row.classList.contains('warn')) return 2;
            if(row.classList.contains('info')) return 1;
            return 0;
          };
          return score(b) - score(a); // 降序
        });
        
        // 将排序后的行移动到顶部（反向插入，保持顺序）
        for(let i = vulnRows.length - 1; i >= 0; i--) {
          outContainer.prepend(vulnRows[i]);
        }
      }
    }
  })
})

// EXP 验证
bind('ex-start', async ()=>{
  const targetBaseUrl = document.getElementById('ex-base')?.value.trim()
  const expValue = document.getElementById('ex-dir')?.value.trim()
  const expPaths = expValue? expValue.split(';').filter(Boolean): []
  const concurrency = parseInt(document.getElementById('ex-conc')?.value)||50
  const timeoutMs = parseInt(document.getElementById('ex-timeout')?.value)||3000
  const provider = document.getElementById('ex-ai-provider')?.value || ''
  const apiKey = document.getElementById('ex-ai-api-key')?.value || ''
  const baseURL = document.getElementById('ex-ai-baseurl')?.value || ''
  const model = document.getElementById('ex-ai-model')?.value || ''
  if(!targetBaseUrl){ alert('请填写基础URL'); return }
  if(!expPaths || expPaths.length===0){ alert('请选择EXP文件'); return }
  try{ const out = document.getElementById('ex-out'); if(out) out.innerHTML = '' }catch{}
  const {taskId} = await api('/ai/exp/python/batch',{provider,apiKey,baseUrl: baseURL,model,targetBaseUrl,timeoutMs,expPaths,concurrency})
  startTask('ex', taskId, (m)=>{
    if(m.type==='start'||m.type==='progress'||m.type==='find'){ setProgress('ex', m.percent, m.progress) }
    if(m.type==='find'){
      const name = m.data?.name || 'EXP'
      const keyInfo = m.data?.keyInfo || ''
      const code = m.data?.python || ''
      const safeName = String(name).replace(/[^\w.-]+/g,'_')
      if(code){ downloadText(`${safeName}.py`, code, 'text/x-python') }
      let right = ''
      if(keyInfo){ right += `<div class="code" style="margin-top:6px;white-space:pre-wrap;font-size:12px">${escapeHtml(keyInfo)}</div>` }
      right += `<div class="code" style="margin-top:10px;white-space:pre-wrap;font-size:12px">${escapeHtml(code)}</div>`
      createRow('ex-out','info', `<strong>${escapeHtml(name)}</strong>`, right)
    } else if(m.type==='scan_log'){
      const name = m.data?.name || 'EXP'
      const msg = m.data?.error || m.data?.status || 'failed'
      let right = `<span class="badge">${escapeHtml(msg)}</span>`
      if (m.data?.keyInfo) {
        right += `<div class="code" style="margin-top:8px;white-space:pre-wrap;font-size:12px">${escapeHtml(m.data.keyInfo)}</div>`
      }
      createRow('ex-out','warn', `<strong>${escapeHtml(name)}</strong>`, right)
    }
    if(m.type==='end'){ setProgress('ex', 100, '生成完成'); createRow('ex-out','info', '生成完成', `<span>${escapeHtml(m.progress||'done')}</span>`) }
  })
})

// Web 探针
bind('wb-start', async ()=>{
  const urls = document.getElementById('wb-urls')?.value.split(/\n+/).map(s=>s.trim()).filter(Boolean)
  if(!urls || urls.length===0){ notify('error','请填写至少一个URL'); return }
  const concurrency = parseInt(document.getElementById('wb-conc')?.value)||50
  const timeoutMs = parseInt(document.getElementById('wb-timeout')?.value)||3000
  // headers json validation
  const headersEl = document.getElementById('wb-headers'); const errEl = document.getElementById('wb-headers-err')
  let headers = {}
  if(headersEl){ headersEl.classList.remove('invalid'); if(errEl) errEl.textContent=''; const raw = headersEl.value.trim(); if(raw){ try{ headers = JSON.parse(raw) } catch(e){ if(errEl) errEl.textContent='JSON 格式错误'; headersEl.classList.add('invalid'); notify('error','请求头JSON格式错误'); return } } }
  const followRedirect = !!document.getElementById('wb-follow')?.checked
  const fetchFavicon = !!document.getElementById('wb-favicon')?.checked
  const fetchRobots = !!document.getElementById('wb-robots')?.checked
  const {taskId} = await api('/scan/webprobe',{urls,concurrency,timeoutMs,headers,followRedirect,fetchFavicon,fetchRobots})
  startTask('wb', taskId, (m)=>{
    if(m.type==='start'||m.type==='progress'){ setProgress('wb', m.percent, m.progress) }
    if(m.type==='find'){
      const tech = (m.data.tech||[]).map(t=>`<span class="badge">${t}</span>`).join(' ')
      const right = [`<span>${m.data.title||''}</span>`, tech, , (typeof m.data.cl==='number'&&m.data.cl>0?`<span class="badge">CL:${m.data.cl}</span>`:''), (m.data.finalUrl&&m.data.finalUrl!==m.data.url?`<span class="badge" title="跳转">→</span>`:'')].filter(Boolean).join(' ')
      createRow('wb-out','info', `${m.data.status} <span class="badge url">${m.data.url}</span>`, right)
      // Add to report
      addWebProbeResult({
        result: m.data
      });
    }
    if(m.type==='end'){ setProgress('wb', 100, m.progress); createRow('wb-out','info', '扫描完成', `<span>${m.progress}</span>`) }
  })
})

// WAF 绕过预设策略定义
const wafPresets = {
  // SQL注入专用策略
  sqli: {
    strategies: 'case,urlencode,doubleencode,tripleencode,space2comment,tab2space,newline2space,split,commentwrap,sqlliteral,sqlchar,nullbyte,capitalize',
    payloads: '1 OR 1=1\n1\' OR \'1\'=\'1\nadmin\'--\n1 UNION SELECT NULL--\n1\' ORDER BY 1--\n1\' UNION SELECT username,password FROM users--\n1\'; DROP TABLE users--\n1\' OR 1=1 --\n" OR 1=1 --\n\' OR \'x\'=\'x'
  },
  // XSS专用策略
  xss: {
    strategies: 'case,urlencode,doubleencode,tripleencode,htmlentity,htmlentitydec,unicode,unicodejs,hex,base64,commentinline,nullbyte,nestedtag,eventhandlervariant,protocolrelative',
    payloads: '<script>alert(1)</script>\n<img src=x onerror=alert(1)>\n<svg onload=alert(1)>\njavascript:alert(1)\n<iframe src=javascript:alert(1)>\n<body onload=alert(1)>\n<input onfocus=alert(1) autofocus>\n<select onfocus=alert(1) autofocus>\n<svg><script>alert(1)</script></svg>\n<scr<script>ipt>alert(1)</scr</script>ipt>'
  },
  // 大小写混淆
  'case': {
    strategies: 'case',
    payloads: ''
  },
  // 编码变形
  encoding: {
    strategies: 'urlencode,doubleencode,tripleencode,htmlentity,htmlentitydec,unicode,unicodejs,hex,base64',
    payloads: ''
  },
  // 注释混淆
  comment: {
    strategies: 'space2comment,tab2space,newline2space,split,commentinline,commentwrap',
    payloads: ''
  },
  // 全策略
  all: {
    strategies: 'case,urlencode,doubleencode,tripleencode,space2comment,tab2space,newline2space,split,commentinline,commentwrap,htmlentity,htmlentitydec,unicode,unicodejs,hex,base64,sqlliteral,sqlchar,nullbyte,capitalize,nestedtag,eventhandlervariant,protocolrelative',
    payloads: ''
  }
}

// 绑定预设按钮
bind('wf-preset-case', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = 'case'
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'all'
  if (p) p.value = ''
})

bind('wf-preset-enc', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = 'urlencode,doubleencode,tripleencode,htmlentity,htmlentitydec,unicode,unicodejs,hex,base64'
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'all'
  if (p) p.value = ''
})

bind('wf-preset-comment', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = 'space2comment,tab2space,newline2space,split,commentinline,commentwrap'
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'all'
  if (p) p.value = ''
})

bind('wf-preset-sql', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = wafPresets.sqli.strategies
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'sqli'
  if (p) p.value = wafPresets.sqli.payloads
})

bind('wf-preset-xss', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = wafPresets.xss.strategies
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'xss'
  if (p) p.value = wafPresets.xss.payloads
})

bind('wf-preset-all', () => {
  const s = document.getElementById('wf-strategies')
  const m = document.getElementById('wf-methods')
  const t = document.getElementById('wf-type')
  const p = document.getElementById('wf-payloads')
  if (s) s.value = wafPresets.all.strategies
  if (m) m.value = 'GET,POST'
  if (t) t.value = 'all'
  if (p) p.value = ''
})

bind('wf-start', async function() {
  var doc = window.document
  var baseUrl = doc.getElementById('wf-base')
  var path = doc.getElementById('wf-path')
  var payloadType = doc.getElementById('wf-type')
  var methodsEl = doc.getElementById('wf-methods')
  var strategiesEl = doc.getElementById('wf-strategies')
  var matchEl = doc.getElementById('wf-match')
  var payloadsEl = doc.getElementById('wf-payloads')
  var concEl = doc.getElementById('wf-conc')
  var timeoutEl = doc.getElementById('wf-timeout')

  baseUrl = baseUrl ? baseUrl.value.trim() : ''
  path = path ? path.value.trim() : ''
  payloadType = payloadType ? (payloadType.value || 'all') : 'all'
  methods = methodsEl ? methodsEl.value.split(',').map(function(s) { return s.trim() }).filter(Boolean) : []
  var strategiesInput = strategiesEl ? strategiesEl.value.trim() : ''
  var strategies = strategiesInput ? strategiesInput.split(',').map(function(s) { return s.trim() }).filter(Boolean) : []
  var match = matchEl ? matchEl.value.trim() : ''
  var userPayloads = payloadsEl ? payloadsEl.value.split('\n').map(function(s) { return s.trim() }).filter(Boolean) : []
  var concurrency = concEl ? (parseInt(concEl.value) || 50) : 50
  var timeoutMs = timeoutEl ? (parseInt(timeoutEl.value) || 4000) : 4000

  if (!baseUrl || !path) {
    alert('请填写基础URL和路径')
    return
  }

  // 根据Payload类型检测和过滤
  const isXSS = (s) => /<\s*script\b|javascript\s*:|alert\s*\(|onerror\s*=|onload\s*=/i.test(s)
  const isSQLi = (s) => /\b(or|union)\b|\bselect\b|--|\/\*|\*\/|#|'|"\b/i.test(s)

  // 处理Payloads
  let payloads = []
  if (userPayloads && userPayloads.length > 0) {
    // 使用用户输入的Payload，并根据类型过滤
    if (payloadType === 'xss') {
      payloads = userPayloads.filter(isXSS)
    } else if (payloadType === 'sqli') {
      payloads = userPayloads.filter(isSQLi)
    } else {
      payloads = userPayloads
    }
  }

  // 如果没有有效Payload且用户没有输入，使用内置库
  if (payloads.length === 0 && (!userPayloads || userPayloads.length === 0)) {
    switch (payloadType) {
      case 'sqli':
        payloads = wafPresets.sqli.payloads.split('\n').filter(Boolean)
        break
      case 'xss':
        payloads = wafPresets.xss.payloads.split('\n').filter(Boolean)
        break
      default:
        // all类型，使用全部内置Payload
        payloads = [...wafPresets.sqli.payloads.split('\n').filter(Boolean), ...wafPresets.xss.payloads.split('\n').filter(Boolean)]
    }
  }

  if (payloads.length === 0) {
    if (payloadType === 'sqli') {
      alert('当前Payload列表中没有检测到SQL注入特征，请检查输入或切换类型')
    } else if (payloadType === 'xss') {
      alert('当前Payload列表中没有检测到XSS特征，请检查输入或切换类型')
    } else {
      alert('请输入要测试的Payload')
    }
    return
  }

  const requestData = {
    baseUrl,
    path,
    payloadType,
    methods,
    strategies,
    match,
    payloads,
    concurrency,
    timeoutMs
  }
  console.log('WAF测试请求:', requestData)

  const { taskId } = await api('/scan/waf', requestData)
  startTask('wf', taskId, (m) => {
    if (m.type === 'start' || m.type === 'progress') { setProgress('wf', m.percent, m.progress) }
    if (m.type === 'find') {
      const method = m.data.method || 'UNKNOWN'
      const payload = m.data.payload || ''
      const variant = m.data.variant || payload || ''
      const status = m.data.status || 0
      const strategies = Array.isArray(m.data.strategies) ? m.data.strategies : []
      const pType = m.data.payloadType || payloadType

      console.log('WAF绕过结果:', { method, payload, variant, status, strategies, payloadType: pType })

      const escapeHtml = (text) => {
        if (!text) return ''
        const div = document.createElement('div')
        div.textContent = String(text)
        return div.innerHTML
      }

      const strategiesText = strategies.length > 0
        ? `<span class="badge" style="background: rgba(0, 229, 255, 0.15); border-color: rgba(0, 229, 255, 0.5); color: #00e5ff; margin-left: 5px;">策略: ${strategies.join(', ')}</span>`
        : ''
      const typeBadge = pType !== 'all' ? `<span class="badge" style="background: rgba(139, 92, 246, 0.2); border-color: rgba(139, 92, 246, 0.6); color: #a78bfa; margin-left: 5px;">${pType === 'sqli' ? 'SQL注入' : 'XSS'}</span>` : ''

      const variantDisplay = `<div style="margin-top: 8px; padding: 10px; background: rgba(17, 24, 39, 0.6); border: 1px solid rgba(16, 185, 129, 0.4); border-left: 3px solid #10b981; border-radius: 6px;">
            <div style="font-weight: bold; color: #10b981; margin-bottom: 6px; font-size: 13px;">✓ 成功绕过的Payload:</div>
            <div class="code" style="background: rgba(0, 0, 0, 0.3); padding: 8px; border-radius: 4px; word-break: break-all; overflow-wrap: break-word; font-family: 'Courier New', monospace; font-size: 13px; white-space: pre-wrap; color: #e5e7eb; border: 1px solid rgba(0, 229, 255, 0.2); max-width: 100%; display: block; box-sizing: border-box;">${escapeHtml(variant)}</div>
          </div>`

      createRow('wf-out', 'success',
        `<div style="width: 100%; max-width: 100%; box-sizing: border-box;">
          <div style="margin-bottom: 8px;">
            <strong>${escapeHtml(method)}</strong>
            <span class="badge" style="margin-left: 8px;">原始: ${escapeHtml(payload)}</span>
            ${typeBadge}
            ${strategiesText}
          </div>
          ${variantDisplay}
        </div>`,
        `<span class="badge" style="background: rgba(16, 185, 129, 0.2); border-color: rgba(16, 185, 129, 0.6); color: #10b981; min-width: 60px; text-align: center;">${status}</span>`)

      addWafScanResult({
        target: baseUrl,
        strategies: strategies.join(','),
        payloadType: pType,
        result: m.data
      })
    }
    if (m.type === 'scan_log') {
      const method = m.data.method || 'UNKNOWN'
      const payload = m.data.payload || ''
      const variant = m.data.variant || ''
      const status = m.data.status || 0
      const strategies = Array.isArray(m.data.strategies) ? m.data.strategies : []
      const strategiesText = strategies.length > 0
        ? `<span class="badge" style="background: rgba(255, 193, 7, 0.15); border-color: rgba(255, 193, 7, 0.5); color: #fbbf24; margin-left: 5px;">策略: ${strategies.join(', ')}</span>`
        : ''
      const escapeHtml = (text) => {
        if (!text) return ''
        const div = document.createElement('div')
        div.textContent = String(text)
        return div.innerHTML
      }
      const variantDisplay = variant
        ? `<div style="margin-top: 8px; padding: 10px; background: rgba(17, 24, 39, 0.6); border: 1px solid rgba(255, 193, 7, 0.35); border-left: 3px solid #fbbf24; border-radius: 6px;">
            <div style="font-weight: bold; color: #fbbf24; margin-bottom: 6px; font-size: 13px;">⚠ 未通过WAF检查的请求:</div>
            <div class="code" style="background: rgba(0, 0, 0, 0.3); padding: 8px; border-radius: 4px; word-break: break-all; overflow-wrap: break-word; font-family: 'Courier New', monospace; font-size: 13px; white-space: pre-wrap; color: #e5e7eb; border: 1px solid rgba(255, 193, 7, 0.2); max-width: 100%; display: block; box-sizing: border-box;">${escapeHtml(variant)}</div>
          </div>`
        : ''
      createRow('wf-out', 'warn',
        `<div style="width: 100%; max-width: 100%; box-sizing: border-box;">
          <div style="margin-bottom: 8px;">
            <strong>${escapeHtml(method)}</strong>
            <span class="badge" style="margin-left: 8px;">原始: ${escapeHtml(payload)}</span>
            ${strategiesText}
          </div>
          ${variantDisplay}
        </div>`,
        `<span class="badge" style="background: rgba(255, 193, 7, 0.2); border-color: rgba(255, 193, 7, 0.6); color: #fbbf24; min-width: 60px; text-align: center;">${status}</span>`)
    }
    if (m.type === 'end') { setProgress('wf', 100, m.progress); createRow('wf-out', 'info', '测试完成', `<span>${m.progress}</span>`) }
  })
})

// initial render lists if page has them
renderSelected('ds-dict-list','ds-dict','ds-file-area')
renderSelected('pc-dir-list','pc-dir','pc-file-area')
renderSelected('ex-dir-list','ex-dir','ex-file-area')

// Report system
const reportData = {
  scanTime: null,
  targets: new Set(),
  portScan: {
    enabled: false,
    target: '',
    scanType: '',
    scanTime: '',
    results: []
  },
  dirScan: {
    enabled: false,
    target: '',
    dictFile: '',
    scanTime: '',
    results: []
  },
  pocScan: {
    enabled: false,
    target: '',
    pocCount: 0,
    scanTime: '',
    results: []
  },
  expScan: {
    enabled: false,
    target: '',
    expCount: 0,
    scanTime: '',
    results: []
  },
  webProbe: {
    enabled: false,
    scanTime: '',
    results: []
  },
  wafScan: {
    enabled: false,
    target: '',
    strategies: '',
    scanTime: '',
    results: []
  },
  shoujiScan: {
    enabled: false,
    target: '',
    scanTime: '',
    jsCount: 0,
    urlCount: 0,
    results: []
  },
  // AI分析结果数据
  // 用于存储AI分析的结果，可在报告导出时包含
  aiAnalysis: {
    enabled: false,      // 是否已执行AI分析
    analysisTime: '',    // AI分析执行时间
    content: ''          // AI分析的完整内容（Markdown格式）
  }
}

// Report data collection functions
function addPortScanResult(data) {
  reportData.portScan.enabled = true
  reportData.portScan.target = data.target || ''
  reportData.portScan.scanType = data.scanType || ''
  reportData.portScan.scanTime = new Date().toLocaleString()
  reportData.targets.add(data.target || '')
  
  if (data.result) {
    reportData.portScan.results.push({
      port: data.result.port,
      status: data.result.status,
      proto: data.result.proto,
      banner: data.result.banner || ''
    })
  }
  
  updateReportSummary()
  saveReportData()
}

function addDirScanResult(data) {
  reportData.dirScan.enabled = true
  reportData.dirScan.target = data.target || ''
  reportData.dirScan.dictFile = data.dictFile || ''
  reportData.dirScan.scanTime = new Date().toLocaleString()
  reportData.targets.add(data.target || '')
  
  if (data.result) {
    reportData.dirScan.results.push({
      path: data.result.path,
      url: data.result.url,
      status: data.result.status,
      location: data.result.location || '',
      length: data.result.length || 0
    })
  }
  
  updateReportSummary()
  saveReportData()
}

function addPocScanResult(data) {
  reportData.pocScan.enabled = true
  reportData.pocScan.target = data.target || ''
  reportData.pocScan.scanTime = new Date().toLocaleString()
  reportData.targets.add(data.target || '')
  
  if (data.result) {
    reportData.pocScan.results.push({
      poc: data.result.poc,
      url: data.result.url,
      status: data.result.status,
      exp: data.result.exp
    })
  }
  
  updateReportSummary()
  saveReportData()
}

function addExpScanResult(data) {
  reportData.expScan.enabled = true
  reportData.expScan.target = data.target || ''
  reportData.expScan.scanTime = new Date().toLocaleString()
  reportData.targets.add(data.target || '')
  
  if (data.result) {
    reportData.expScan.results.push({
      name: data.result.name,
      matchedSteps: data.result.matchedSteps,
      lastStatus: data.result.lastStatus
    })
  }
  
  updateReportSummary()
  saveReportData()
}

function addWebProbeResult(data) {
  reportData.webProbe.enabled = true
  reportData.webProbe.scanTime = new Date().toLocaleString()
  
  if (data.result) {
    reportData.webProbe.results.push({
      url: data.result.url,
      finalUrl: data.result.finalUrl,
      status: data.result.status,
      title: data.result.title || '',
      tech: data.result.tech || [],
      proto: data.result.proto || '',
      cl: data.result.cl || 0
    })
    reportData.targets.add(data.result.url)
  }
  
  updateReportSummary()
  saveReportData()
}

function addWafScanResult(data) {
  reportData.wafScan.enabled = true
  reportData.wafScan.target = data.target || ''
  reportData.wafScan.strategies = data.strategies || ''
  reportData.wafScan.payloadType = data.payloadType || 'all'
  reportData.wafScan.scanTime = new Date().toLocaleString()
  reportData.targets.add(data.target || '')

  if (data.result) {
    reportData.wafScan.results.push({
      method: data.result.method,
      payload: data.result.payload,
      variant: data.result.variant,
      status: data.result.status,
      payloadType: data.result.payloadType || data.payloadType || 'all'
    })
  }

  updateReportSummary()
  saveReportData()
}

function addShoujiScanResult(data) {
  reportData.shoujiScan.enabled = true
  reportData.shoujiScan.target = data.target || ''
  reportData.shoujiScan.scanTime = new Date().toLocaleString()
  reportData.shoujiScan.jsCount = data.jsCount || 0
  reportData.shoujiScan.urlCount = data.urlCount || 0
  reportData.targets.add(data.target || '')
  
  if (data.results && Array.isArray(data.results)) {
    // 如果results是数组，直接使用；否则清空并重新添加
    reportData.shoujiScan.results = data.results.slice(0, 1000) // 限制最多1000条
  } else if (data.result) {
    // 确保 results 是数组
    if (!Array.isArray(reportData.shoujiScan.results)) {
      reportData.shoujiScan.results = []
    }
    reportData.shoujiScan.results.push({
      url: data.result.url || '',
      status: data.result.status || '',
      size: data.result.size || '',
      title: data.result.title || '',
      redirect: data.result.redirect || '',
      source: data.result.source || '',
      kind: data.result.kind || 'other'
    })
  }
  
  // 确保 results 始终是数组
  if (!Array.isArray(reportData.shoujiScan.results)) {
    reportData.shoujiScan.results = []
  }
  
  updateReportSummary()
  saveReportData()
  
  // 如果当前在报告页面，立即重新渲染相关部分
  if (window.location.pathname.includes('report.html')) {
    if (typeof renderReportSections === 'function') {
      renderReportSections()
    }
  }
}

function updateReportSummary() {
  if (!reportData.scanTime) {
    reportData.scanTime = new Date().toLocaleString()
  }
  
  // Update summary values
  const summary = {
    scanTime: reportData.scanTime,
    targetCount: reportData.targets.size,
    portCount: reportData.portScan.results.length,
    dirCount: reportData.dirScan.results.length,
    vulnCount: reportData.pocScan.results.length + reportData.expScan.results.length,
    webCount: reportData.webProbe.results.length,
    shoujiCount: reportData.shoujiScan.results.length
  }
  
  // Update UI if on report page
  if (window.location.pathname.includes('report.html')) {
    document.getElementById('scan-time').textContent = summary.scanTime
    document.getElementById('target-count').textContent = summary.targetCount
    document.getElementById('port-count').textContent = summary.portCount
    document.getElementById('dir-count').textContent = summary.dirCount
    document.getElementById('vuln-count').textContent = summary.vulnCount
    document.getElementById('web-count').textContent = summary.webCount
    document.getElementById('shouji-count').textContent = summary.shoujiCount
  }
}

function saveReportData() {
  try{
    // Serialize reportData but convert Set to Array for storage
    const copy = JSON.parse(JSON.stringify(reportData, (k,v)=>{
      if(k==='targets' && v instanceof Set) return Array.from(v)
      return v
    }))
    // If reportData.targets is a Set, ensure it's an array in copy
    if(reportData.targets instanceof Set){ copy.targets = Array.from(reportData.targets) }
    localStorage.setItem('neonscan-report', JSON.stringify(copy))
  }catch(e){ console.warn('saveReportData failed', e) }
}

// 暴露到window对象，供AI分析页面使用
// 这样AI分析页面可以直接访问和更新报告数据
window.reportData = reportData;
window.saveReportData = saveReportData;

function loadReportData() {
  const saved = localStorage.getItem('neonscan-report')
  if (saved) {
    try{
      const data = JSON.parse(saved)
      // copy fields except targets (we'll convert targets safely)
      Object.keys(data).forEach(k=>{
        if(k==='targets') return
        reportData[k] = data[k]
      })
      // Normalize targets: accept Array or stringified Array
      let targetsArr = []
      if (Array.isArray(data.targets)) targetsArr = data.targets
      else if (typeof data.targets === 'string') {
        try{ targetsArr = JSON.parse(data.targets) }catch{ targetsArr = [] }
        if(!Array.isArray(targetsArr)) targetsArr = []
      } else if (data.targets && typeof data.targets === 'object' && data.targets.constructor===Object) {
        // maybe saved as object from earlier bad serialization, ignore
        targetsArr = Object.keys(data.targets)
      }
      reportData.targets = new Set(targetsArr||[])
      
      // 确保 shoujiScan.results 是一个数组
      if (reportData.shoujiScan && !Array.isArray(reportData.shoujiScan.results)) {
        reportData.shoujiScan.results = []
      }
      
      // 确保 aiAnalysis 字段存在（向后兼容）
      // 如果从旧版本数据加载，可能没有aiAnalysis字段，需要初始化
      if (!reportData.aiAnalysis) {
        reportData.aiAnalysis = {
          enabled: false,
          analysisTime: '',
          content: ''
        }
      }
      
      updateReportSummary()
      
      // 调试信息
      if (reportData.shoujiScan && reportData.shoujiScan.enabled) {
        console.log('ShoujiScan data loaded:', {
          enabled: reportData.shoujiScan.enabled,
          target: reportData.shoujiScan.target,
          resultsCount: reportData.shoujiScan.results ? reportData.shoujiScan.results.length : 0
        })
      }
    }catch(e){ console.warn('loadReportData failed', e); }
  }
}

function clearReportData() {
  Object.keys(reportData).forEach(key => {
    if (key === 'targets') {
      reportData[key] = new Set()
    } else if (typeof reportData[key] === 'object' && reportData[key] !== null) {
      if (Array.isArray(reportData[key])) {
        reportData[key] = []
      } else {
        Object.keys(reportData[key]).forEach(subKey => {
          if (Array.isArray(reportData[key][subKey])) {
            reportData[key][subKey] = []
          } else {
            reportData[key][subKey] = ''
          }
        })
      }
    } else {
      reportData[key] = null
    }
  })
  reportData.scanTime = null
  saveReportData()
  updateReportSummary()
}

// Load report data on page load
try{ if(typeof loadReportData==='function'){ loadReportData() } }catch(e){ console.warn('loadReportData init failed', e) }

// restore running tasks (reconnect SSE) if any
(function restoreRunning(){
  try{
    const saved = JSON.parse(localStorage.getItem('neonscan-running')||'{}')
    Object.keys(saved).forEach(key=>{
      const id = saved[key]
      if(id){
        // try to reconnect; attach simple handler that updates progress
        startTask(key, id, (m)=>{
          // best-effort: if on report page, refresh summary
          if(m.type==='find' || m.type==='end' || m.type==='progress'){
            updateReportSummary()
          }
        })
      }
    })
  }catch(e){ console.warn('restore running tasks failed', e) }
})()

// Global error handling
window.addEventListener('error', (e) => {
  console.error('JavaScript error:', e.error)
})

// Debug info
console.log('NeonScan app.js loaded')

// Shell page handlers removed by request

console.log('Available functions:', {
  bind: typeof bind,
  adjustNumber: typeof adjustNumber,
  createRow: typeof createRow,
  setProgress: typeof setProgress
})



// Ensure DOM is ready before binding buttons
function ensureDOMReady() {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      console.log('DOM ready, binding buttons...')
      bindAllButtons()
    })
  } else {
    console.log('DOM already ready, binding buttons...')
    bindAllButtons()
  }
}

function bindAllButtons() {
  // Attempt to bind any pending bindings saved earlier
  Object.keys(pendingBindings).forEach(id=>{
    const el = document.getElementById(id)
    if(el){ el.onclick = pendingBindings[id]; console.log(`Late-bound button ${id}`); delete pendingBindings[id] }
  })
  console.log('All buttons should be bound now')
}

// Report page functionality
function initReportPage() {
  // 确保先加载数据
  loadReportData()
  updateReportSummary()
  renderReportSections()
  
  // Bind report controls
  bind('clear-report', () => {
    if (confirm('确定要清空所有扫描报告数据吗？')) {
      clearReportData()
      renderReportSections()
    }
  })
  
  bind('export-report', () => {
    exportReport()
  })
  
  bind('refresh-report', () => {
    loadReportData()
    renderReportSections()
  })
}

function renderReportSections() {
  const sections = [
    { key: 'portScan', id: 'port-scan-section', title: '端口扫描结果' },
    { key: 'dirScan', id: 'dir-scan-section', title: '目录扫描结果' },
    { key: 'pocScan', id: 'poc-scan-section', title: 'POC 扫描结果' },
    { key: 'expScan', id: 'exp-scan-section', title: 'EXP 验证结果' },
    { key: 'webProbe', id: 'web-probe-section', title: 'Web 探针结果' },
    { key: 'wafScan', id: 'waf-scan-section', title: 'WAF绕过结果' },
    { key: 'shoujiScan', id: 'shouji-scan-section', title: '解包与JS信息收集结果' }
  ]
  
  let hasData = false
  
  sections.forEach(section => {
    const data = reportData[section.key]
    const sectionEl = document.getElementById(section.id)
    
    if (!sectionEl) {
      console.warn(`Section element not found: ${section.id}`)
      return
    }
    
    if (data && data.enabled && data.results && Array.isArray(data.results) && data.results.length > 0) {
      hasData = true
      sectionEl.style.display = 'block'
      renderSectionContent(section.key, sectionEl)
    } else {
      sectionEl.style.display = 'none'
    }
  })
  
  // Show/hide empty state
  const emptyEl = document.getElementById('report-empty')
  emptyEl.style.display = hasData ? 'none' : 'block'
}

function renderSectionContent(sectionKey, sectionEl) {
  const data = reportData[sectionKey]
  if (!data || !data.results || !Array.isArray(data.results)) {
    console.warn(`renderSectionContent: Invalid data for ${sectionKey}`, data)
    return
  }
  
  // 优先使用 id 查找，因为 HTML 中定义了 id
  let resultsEl = null
  if (sectionKey === 'shoujiScan') {
    resultsEl = document.getElementById('shouji-results')
  } else {
    resultsEl = sectionEl.querySelector('.results-table')
  }
  
  if (!resultsEl) {
    console.warn(`renderSectionContent: Cannot find results element for ${sectionKey}`)
    return
  }
  
  // Fill section headers if present
  try{
    if(sectionKey==='portScan'){
      document.getElementById('port-target').textContent = reportData.portScan.target||'-'
      document.getElementById('port-time').textContent = reportData.portScan.scanTime||'-'
      document.getElementById('port-type').textContent = (reportData.portScan.scanType||'-').toUpperCase()
    } else if(sectionKey==='dirScan'){
      document.getElementById('dir-target').textContent = reportData.dirScan.target||'-'
      document.getElementById('dir-time').textContent = reportData.dirScan.scanTime||'-'
      document.getElementById('dir-dict').textContent = reportData.dirScan.dictFile||'-'
    } else if(sectionKey==='pocScan'){
      document.getElementById('poc-target').textContent = reportData.pocScan.target||'-'
      document.getElementById('poc-time').textContent = reportData.pocScan.scanTime||'-'
      document.getElementById('poc-count').textContent = String(reportData.pocScan.results.length)
    } else if(sectionKey==='expScan'){
      document.getElementById('exp-target').textContent = reportData.expScan.target||'-'
      document.getElementById('exp-time').textContent = reportData.expScan.scanTime||'-'
      document.getElementById('exp-count').textContent = String(reportData.expScan.results.length)
    } else if(sectionKey==='webProbe'){
      document.getElementById('web-time').textContent = reportData.webProbe.scanTime||'-'
      document.getElementById('web-target-count').textContent = String(reportData.webProbe.results.length)
    } else if(sectionKey==='wafScan'){
      document.getElementById('waf-target').textContent = reportData.wafScan.target||'-'
      document.getElementById('waf-time').textContent = reportData.wafScan.scanTime||'-'
      document.getElementById('waf-strategies').textContent = reportData.wafScan.strategies||'-'
    } else if(sectionKey==='shoujiScan'){
      const shoujiTargetEl = document.getElementById('shouji-target')
      const shoujiTimeEl = document.getElementById('shouji-time')
      const shoujiJsCountEl = document.getElementById('shouji-js-count')
      const shoujiUrlCountEl = document.getElementById('shouji-url-count')
      if(shoujiTargetEl) shoujiTargetEl.textContent = reportData.shoujiScan.target||'-'
      if(shoujiTimeEl) shoujiTimeEl.textContent = reportData.shoujiScan.scanTime||'-'
      if(shoujiJsCountEl) shoujiJsCountEl.textContent = String(reportData.shoujiScan.jsCount||0)
      if(shoujiUrlCountEl) shoujiUrlCountEl.textContent = String(reportData.shoujiScan.urlCount||0)
    }
  }catch{}
  
  // Clear existing results
  resultsEl.innerHTML = ''
  
  // Render results
  data.results.forEach(result => {
    const item = document.createElement('div')
    item.className = 'result-item'
    
    let leftContent = ''
    let rightContent = ''
    
    switch(sectionKey) {
      case 'portScan':
        leftContent = `<div class="result-icon open"></div><span>端口 ${result.port}</span>`
        rightContent = `<span class="badge">${result.proto.toUpperCase()}</span>${result.banner ? `<span class="badge" title="${result.banner}">Banner</span>` : ''}`
        break
      case 'dirScan':
        leftContent = `<div class="result-icon info"></div><span>${result.url}</span>`
        rightContent = `<span class="badge">${result.status}</span>${result.length ? `<span class="badge">${result.length}B</span>` : ''}`
        break
      case 'pocScan':
        leftContent = `<div class=\"result-icon vulnerable\"></div><span>${result.poc}</span>`
        const isSuspect = String(result.status).toLowerCase()=== 'suspect'
        rightContent = `${isSuspect?'<span class=\"badge warn\">suspect</span>':`<span class=\"badge\">${result.status}</span>`}<span class=\"code\">${result.exp}</span>`
        break
      case 'expScan':
        leftContent = `<div class="result-icon success"></div><span>${result.name}</span>`
        rightContent = `<span class="badge">Steps: ${result.matchedSteps}</span><span class="badge">Status: ${result.lastStatus}</span>`
        break
      case 'webProbe':
        leftContent = `<div class="result-icon info"></div><span>${result.url}</span>`
        rightContent = `<span class="badge">${result.status}</span>${result.title ? `<span>${result.title}</span>` : ''}`
        break
      case 'wafScan':
        const variantEscaped = (result.variant || '').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        const payloadEscaped = (result.payload || '').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        leftContent = `<div class="result-icon success"></div><span>${result.method} ${payloadEscaped}</span>`
        rightContent = `<span class="code">${variantEscaped}</span><span class="badge">${result.status}</span>`
        break
      case 'shoujiScan':
        const kindBadge = result.kind === 'js' ? 'js' : (result.kind === 'api' ? 'api' : 'other')
        const statusBadge = result.status ? `<span class="badge">${result.status}</span>` : ''
        // 格式化文件大小显示
        let sizeBadge = ''
        if (result.size) {
          const sizeNum = parseInt(result.size)
          if (!isNaN(sizeNum)) {
            let sizeStr = result.size
            if (sizeNum >= 1024*1024) sizeStr = (sizeNum/(1024*1024)).toFixed(2) + 'MB'
            else if (sizeNum >= 1024) sizeStr = (sizeNum/1024).toFixed(2) + 'KB'
            else sizeStr = sizeNum + 'B'
            sizeBadge = `<span class="badge">${sizeStr}</span>`
          } else {
            sizeBadge = `<span class="badge">${result.size}</span>`
          }
        }
        const titleText = result.title ? (result.title.length > 30 ? result.title.substring(0, 30) + '...' : result.title) : ''
        leftContent = `<div class="result-icon info"></div><span class="badge">${kindBadge}</span> <span class="badge url" title="${(result.url || '').replace(/"/g, '&quot;')}">${result.url || ''}</span>`
        rightContent = `${statusBadge}${sizeBadge}${titleText ? `<span title="${(result.title || '').replace(/"/g, '&quot;')}">${titleText}</span>` : ''}`
        break
    }
    
    item.innerHTML = `
      <div class="result-left">${leftContent}</div>
      <div class="result-right">${rightContent}</div>
    `
    resultsEl.appendChild(item)
  })
}

function exportReport() {
  // Build Markdown report from analyzed useful info
  const lines = []
  lines.push('# NeonScan 扫描报告')
  lines.push('')
  const ts = reportData.scanTime || new Date().toLocaleString()
  lines.push(`- 生成时间: ${ts}`)
  const summary = {
    targetCount: reportData.targets.size,
    portCount: reportData.portScan.results.length,
    dirCount: reportData.dirScan.results.length,
    vulnCount: reportData.pocScan.results.length + reportData.expScan.results.length,
    webCount: reportData.webProbe.results.length,
    shoujiCount: (reportData.shoujiScan && reportData.shoujiScan.results) ? reportData.shoujiScan.results.length : 0,
    wafCount: reportData.wafScan.results.length
  }
  lines.push(`- 目标数量: ${summary.targetCount}`)
  lines.push(`- 发现端口: ${summary.portCount}`)
  lines.push(`- 发现目录: ${summary.dirCount}`)
  lines.push(`- 漏洞数量: ${summary.vulnCount}`)
  lines.push(`- Web应用: ${summary.webCount}`)
  lines.push(`- JS/URL收集: ${summary.shoujiCount}`)
  lines.push(`- WAF绕过测试: ${summary.wafCount}`)
  lines.push('')

  const interestingStatus = s => [200,301,302,401,403].includes(Number(s)) || typeof s==='string'
  const md = s => String(s||'').replace(/[\\`*_{}\[\]()#+\-.!|]/g, '\\$&')

  if (reportData.portScan.enabled && reportData.portScan.results.length) {
    lines.push('## 端口扫描')
    lines.push(`- 目标: ${md(reportData.portScan.target)}  类型: ${(reportData.portScan.scanType||'').toUpperCase()}`)
    reportData.portScan.results.forEach(r=>{
      const item = `${r.port}/${(r.proto||'').toUpperCase()}${r.banner?` - ${md(r.banner)}`:''}`
      lines.push(`- ${item}`)
    })
    lines.push('')
  }

  if (reportData.dirScan.enabled && reportData.dirScan.results.length) {
    lines.push('## 目录扫描')
    lines.push(`- 目标: ${md(reportData.dirScan.target)}  字典: ${md(reportData.dirScan.dictFile)}`)
    reportData.dirScan.results.filter(r=>interestingStatus(r.status)).slice(0,500).forEach(r=>{
      const extras = []
      if (r.length) extras.push(`len=${r.length}`)
      if (r.location) extras.push(`loc=${md(r.location)}`)
      lines.push(`- [${r.status}] ${md(r.url)} ${extras.length?`(${extras.join(', ')})`:''}`)
    })
    lines.push('')
  }

  if (reportData.pocScan.enabled && reportData.pocScan.results.length) {
    lines.push('## POC 扫描')
    lines.push(`- 目标: ${md(reportData.pocScan.target)}  条目: ${reportData.pocScan.results.length}`)
    reportData.pocScan.results.forEach(r=>{
      lines.push(`- ${md(r.poc)} | ${md(r.url)} | 状态: ${md(r.status)}${r.exp?` | exp: \`${md(r.exp)}\``:''}`)
    })
    lines.push('')
  }

  if (reportData.expScan.enabled && reportData.expScan.results.length) {
    lines.push('## EXP 验证')
    lines.push(`- 目标: ${md(reportData.expScan.target)}  条目: ${reportData.expScan.results.length}`)
    reportData.expScan.results.forEach(r=>{
      lines.push(`- ${md(r.name)} | Steps: ${r.matchedSteps} | Status: ${md(r.lastStatus)}`)
    })
    lines.push('')
  }

  if (reportData.webProbe.enabled && reportData.webProbe.results.length) {
    lines.push('## Web 探针')
    reportData.webProbe.results.forEach(r=>{
      const parts = [`[${r.status}] ${md(r.url)}`]
      if (r.title) parts.push(md(r.title))
      if (Array.isArray(r.tech) && r.tech.length) parts.push('tech: '+md(r.tech.join(', ')))
      if (r.cl) parts.push(`CL=${r.cl}`)
      if (r.finalUrl && r.finalUrl!==r.url) parts.push(`→ ${md(r.finalUrl)}`)
      lines.push(`- ${parts.join(' | ')}`)
    })
    lines.push('')
  }

  if (reportData.wafScan.enabled && reportData.wafScan.results.length) {
    lines.push('## WAF 绕过')
    lines.push(`- 目标: ${md(reportData.wafScan.target)}`)
    lines.push(`- Payload类型: ${md(reportData.wafScan.payloadType === 'sqli' ? 'SQL注入' : reportData.wafScan.payloadType === 'xss' ? 'XSS跨站脚本' : '全部')}`)
    lines.push(`- 策略: ${md(reportData.wafScan.strategies)}`)
    reportData.wafScan.results.forEach(r => {
      const pType = r.payloadType === 'sqli' ? '[SQLi]' : r.payloadType === 'xss' ? '[XSS]' : ''
      lines.push(`- ${pType} ${md(r.method)} ${md(r.payload)} | 变体: \`${md(r.variant)}\` | 状态: ${md(r.status)}`)
    })
    lines.push('')
  }

  if (reportData.shoujiScan && reportData.shoujiScan.enabled && reportData.shoujiScan.results && reportData.shoujiScan.results.length) {
    lines.push('## 解包与JS信息收集')
    lines.push(`- 目标: ${md(reportData.shoujiScan.target || '')}`)
    lines.push(`- 扫描时间: ${md(reportData.shoujiScan.scanTime || '')}`)
    lines.push(`- JS文件数量: ${reportData.shoujiScan.jsCount || 0}`)
    lines.push(`- URL数量: ${reportData.shoujiScan.urlCount || 0}`)
    lines.push(`- 总条目数: ${reportData.shoujiScan.results.length}`)
    lines.push('')
    lines.push('### 收集到的URL和文件:')
    reportData.shoujiScan.results.slice(0, 1000).forEach(r=>{
      const parts = []
      if (r.kind) parts.push(`类型: ${r.kind}`)
      if (r.status) parts.push(`状态: ${r.status}`)
      if (r.size) parts.push(`大小: ${r.size}`)
      if (r.title) parts.push(`标题: ${md(r.title)}`)
      if (r.redirect && r.redirect !== r.url) parts.push(`跳转: ${md(r.redirect)}`)
      if (r.source) parts.push(`来源: ${r.source}`)
      const item = parts.length 
        ? `${md(r.url || '')} (${parts.join(', ')})`
        : `${md(r.url || '')}`
      lines.push(`- ${item}`)
    })
    if (reportData.shoujiScan.results.length > 1000) {
      lines.push(`- ... 还有 ${reportData.shoujiScan.results.length - 1000} 条结果未显示`)
    }
    lines.push('')
  }

  // AI分析结果（导出时包含）
  // 如果已执行AI分析，将分析结果添加到导出报告中
  if (reportData.aiAnalysis && reportData.aiAnalysis.enabled && reportData.aiAnalysis.content) {
    lines.push('## AI 安全分析')
    lines.push(`- 分析时间: ${md(reportData.aiAnalysis.analysisTime || '')}`)
    lines.push('')
    // 将AI分析内容按行分割并添加，保持原始格式（包括Markdown格式）
    const analysisLines = reportData.aiAnalysis.content.split('\n')
    analysisLines.forEach(line => {
      lines.push(line)
    })
    lines.push('')
  }

  const content = lines.join('\n')
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `neonscan-report-${new Date().toISOString().slice(0,19).replace(/:/g,'-')}.md`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

// Initialize report page when DOM is ready
if (window.location.pathname.includes('report.html')) {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initReportPage)
  } else {
    initReportPage()
  }
}

// Pending EXP helpers
function getPendingExps(){
  try{ return JSON.parse(localStorage.getItem('neonscan-pending-exps')||'[]') }catch{ return [] }
}
function setPendingExps(list){
  try{ localStorage.setItem('neonscan-pending-exps', JSON.stringify(list||[])) }catch{}
}
function generateExpFromPocPayload(payload){
  try{ return JSON.parse(decodeURIComponent(payload)) }catch{ return null }
}
window.generateExpFromPoc = (payload)=>{
  const obj = generateExpFromPocPayload(payload); if(!obj) return;
  const {data, baseUrl} = obj;
  let method = 'GET'
  let headers = {}
  let body = ''
  if(data && data.req){
    method = String(data.req.method||'GET').toUpperCase()
    headers = (data.req.headers && typeof data.req.headers === 'object') ? data.req.headers : {}
    body = String(data.req.body||'')
  } else {
    const curl = String(data.exp||'')
    const m = curl.match(/-X\s+(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)/i)
    if(m) method = m[1].toUpperCase()
  }
  // derive path relative to baseUrl if possible
  let path = ''
  if(data && data.req && data.req.path){
    path = String(data.req.path||'')
  }
  try{
    const u = new URL(data.url)
    const b = new URL(baseUrl)
    if(!path){
      if(u.origin===b.origin){ path = u.pathname + (u.search||'') }
      else { path = u.href.replace(b.origin, '') }
    }
  }catch{ path = data.url }

  let statusList = []
  if(typeof data.status === 'number') statusList = [data.status]
  else {
    const s = parseInt(String(data.status||''), 10)
    if(!Number.isNaN(s)) statusList = [s]
  }
  if(!statusList.length) statusList = [200]

  const validate = { status: statusList }
  const mEcho = String(body||'').match(/echo(?:%20|\s+)'?([A-Za-z0-9_]{4,80})'?/i)
  if(mEcho && mEcho[1]) validate.bodyContains = [mEcho[1]]

  // If this looks like ThinkPHP 5.0.x captcha RCE, replace the hardcoded command with a placeholder
  // so generated python can support --cmd/--shell.
  if(method === 'POST' && /index\.php\?s=captcha/i.test(path) && /server\[REQUEST_METHOD\]=/i.test(body)) {
    body = body.replace(/server\[REQUEST_METHOD\]=[^&]*/i, 'server[REQUEST_METHOD]={{cmd_urlenc}}')
    validate.bodyContains = validate.bodyContains || ['NEONSCAN_OK']
  }

  const spec = {
    name: data.poc || 'Generated EXP',
    steps: [{
      method,
      path,
      body,
      headers,
      validate,
      retry: 0,
      retryDelayMs: 0
    }]
  }
  const list = getPendingExps(); list.push(spec); setPendingExps(list)
  alert('已生成EXP并加入待生成列表')
  // 不再自动跳转，用户可以在EXP页面查看待验证列表
}
// Delegate click on generated buttons
(function(){
  const out = document.getElementById('pc-out')
  if(out){
    out.addEventListener('click', (e)=>{
      const btn = e.target.closest('.gen-exp-btn')
      if(btn){ const payload = btn.getAttribute('data-payload'); if(payload){ try{ window.generateExpFromPoc(payload) }catch{} } }
    })
  }
})()

// Delegate click on open shell buttons - FIXED
;(function(){
  const out = document.getElementById('pc-out')
  if(out){
    out.addEventListener('click', (e)=>{
      // This handler seems redundant or broken in original code (payload undefined). 
      // The generation handler is already defined above (line 1480+).
      // Keeping empty to prevent errors.
    })
  }
})()

function renderPendingExps(){
  const c = document.getElementById('exp-pending'); if(!c) return;
  const list = getPendingExps();
  c.innerHTML = ''
  if(!list.length){ c.innerHTML = '<div class="empty">暂无待生成EXP</div>'; return }
  list.forEach((spec, idx)=>{
    const row = document.createElement('div'); row.className = 'row'
    const left = document.createElement('div'); left.className = 'left'; left.innerHTML = `<strong>${spec.name}</strong> <span class="badge">${spec.steps?.[0]?.method||''} ${spec.steps?.[0]?.path||''}</span>`
    const right = document.createElement('div'); right.className = 'right'
    const btn = document.createElement('button'); btn.className = 'btn'; btn.textContent = '开始生成'
    btn.onclick = async ()=>{
      const targetBaseUrl = document.getElementById('ex-base')?.value.trim()
      const timeoutMs = parseInt(document.getElementById('ex-timeout')?.value)||3000
      if(!targetBaseUrl){ alert('请填写基础URL'); return }
      const provider = document.getElementById('ex-ai-provider')?.value || ''
      const apiKey = document.getElementById('ex-ai-api-key')?.value || ''
      const baseURL = document.getElementById('ex-ai-baseurl')?.value || ''
      const model = document.getElementById('ex-ai-model')?.value || ''
      try{
        const resp = await api('/ai/exp/python', { provider, apiKey, baseUrl: baseURL, model, targetBaseUrl, timeoutMs, exp: spec })
        const code = resp?.python || ''
        const keyInfo = resp?.keyInfo || ''
        const safeName = String(spec.name||'exp').replace(/[^\w.-]+/g,'_')
        if(code){ downloadText(`${safeName}.py`, code, 'text/x-python') }
        let right = ''
        if(keyInfo){ right += `<div class="code" style="margin-top:6px;white-space:pre-wrap;font-size:12px">${escapeHtml(keyInfo)}</div>` }
        right += `<div class="code" style="margin-top:10px;white-space:pre-wrap;font-size:12px">${escapeHtml(code)}</div>`
        createRow('ex-out','info', `<strong>${escapeHtml(resp?.name||spec.name||'EXP')}</strong>`, right)
      }catch(e){
        alert('生成失败：'+(e?.message||e))
      }
    }
    right.appendChild(btn);
    row.appendChild(left); row.appendChild(right);
    c.appendChild(row)
  })
}

// Final DOM-ready safety: ensure persisted forms restored and pending bindings applied
function bindAllButtons(){
  // bind pendingBindings map
  try{
    Object.keys(pendingBindings||{}).forEach(id=>{
      const el = document.getElementById(id)
      if(el){ el.onclick = pendingBindings[id]; console.log(`Late-bound button ${id}`); delete pendingBindings[id] }
    })
  }catch(e){ console.warn('bindAllButtons bind pending failed', e) }
  // also apply window.__bindings
  try{
    const map = window.__bindings || {}
    Object.keys(map).forEach(id=>{
      const el = document.getElementById(id)
      if(el && !el.onclick){ el.onclick = map[id]; console.log('applied __bindings', id) }
    })
  }catch(e){ console.warn('bindAllButtons apply __bindings failed', e) }
  console.log('bindAllButtons completed')
}

(function finalInit(){
  function applySavedBindings(){
    try{
      const map = window.__bindings || {}
      Object.keys(map).forEach(id=>{
        const el = document.getElementById(id)
        if(el) { el.onclick = map[id]; console.log('applied saved binding', id) }
      })
    }catch(e){ console.warn('applySavedBindings failed', e) }
  }

  function applyFallbackDirectBindings(){
    const ids = ['ps-start','ps-stop','ds-start','pc-start','ex-start','wb-start','wf-start']
    ids.forEach(id=>{
      try{
        const el = document.getElementById(id)
        if(!el) return
        if(el.onclick) return
        // try pendingBindings / __bindings
        const fn = (window.pendingBindings||{})[id] || (window.__bindings||{})[id]
        if(fn){ el.onclick = fn; console.log('fallback bind via maps', id); return }
        // try named handler: ps-start -> handlePsStart
        const parts = id.split('-').map((s,i)=> i===0? s : s.charAt(0).toUpperCase()+s.slice(1))
        const handlerName = 'handle' + parts.map(p=>p.charAt(0).toUpperCase()+p.slice(1)).join('')
        const named = window[handlerName]
        if(typeof named === 'function'){ el.onclick = named; console.log('fallback bind via named handler', id); return }
      }catch(e){ console.warn('fallback bind failed for', id, e) }
    })
  }

  function initPendingExpUI(){ try{ renderPendingExps() }catch(e){ console.warn('renderPendingExps failed', e) } }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', ()=>{ restoreFormState(); attachPersistOnChange(); applySavedBindings(); try{ bindAllButtons() }catch{}; applyFallbackDirectBindings(); initPendingExpUI() })
  } else {
    try{ restoreFormState(); attachPersistOnChange(); applySavedBindings(); bindAllButtons(); applyFallbackDirectBindings(); initPendingExpUI() }catch(e){ console.warn(e) }
  }
})()

// Delegate click on open shell buttons
;(function(){
  const out = document.getElementById('pc-out')
  if(out){
    out.addEventListener('click', (e)=>{

      if(!payload) return
      try{
        const obj = JSON.parse(decodeURIComponent(payload))
        const {data, baseUrl} = obj
        // derive method from curl
        let method = 'GET'
        const curl = String(data.exp||'')
        const m = curl.match(/-X\s+(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)/i)
        if(m) method = m[1].toUpperCase()
        // derive path relative to baseUrl if possible
        let path = ''
        try{
          const u = new URL(data.url)
          const b = new URL(baseUrl)
          if(u.origin===b.origin){ path = u.pathname + (u.search||'') }
          else { path = u.href.replace(b.origin, '') }
        }catch{ path = data.url }
        
      }catch{}
    })
  }
})()
