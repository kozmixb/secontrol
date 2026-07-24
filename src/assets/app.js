const $=s=>document.querySelector(s), $$=s=>document.querySelectorAll(s);
const initialTheme=localStorage.getItem("secontrol-theme")||"dark";
document.documentElement.dataset.theme=initialTheme;
let agents=[],sshKeys=[],fleetContainers=[],connectionVerified=false,routeReady=false;
const containerUpdateCache=new Map();
const fmtBytes=n=>{if(!n)return "—";const u=["B","KB","MB","GB","TB"];let i=0;while(n>=1024&&i<u.length-1){n/=1024;i++}return `${n.toFixed(i>2?1:0)} ${u[i]}`};
const pct=(used,total)=>total?Math.round(used/total*100):0;
const ago=value=>{if(!value)return "Never";const s=Math.max(0,(Date.now()-new Date(value))/1000);if(s<60)return "Just now";if(s<3600)return `${Math.floor(s/60)}m ago`;if(s<86400)return `${Math.floor(s/3600)}h ago`;return `${Math.floor(s/86400)}d ago`};
const compactPorts=value=>{const ports=[];for(const entry of (value||"").split(",")){let part=entry.trim();if(!part)continue;const udp=/\/udp$/i.test(part);part=part.replace(/\/(?:tcp|udp)$/i,"");if(part.includes("->")){let source=part.split("->")[0];source=source.startsWith("[")?source.slice(source.indexOf("]:")+2):source.split(":").pop();part=source}const label=part+(udp?"/udp":"");if(part&&!ports.includes(label))ports.push(label)}return ports.join(", ")||"—"};
async function api(url,opts={}){const r=await fetch(url,{headers:{"Content-Type":"application/json"},...opts});if(!r.ok){const e=await r.json().catch(()=>({error:r.statusText}));throw Error(e.error)}return r.status===204?null:r.json()}
function toast(message){const t=$("#toast");t.textContent=message;t.classList.add("show");setTimeout(()=>t.classList.remove("show"),2600)}
async function load(){
  try{const [overview,list,keys,containers]=await Promise.all([api("/api/overview"),api("/api/agents"),api("/api/ssh-keys"),api("/api/containers")]);agents=list;sshKeys=keys;fleetContainers=containers;renderOverview(overview);renderMachines();renderContainers();renderKeys();if(!routeReady){routeReady=true;routeFromLocation()}}catch(e){toast(e.message)}
}
function renderOverview(o){
  $("#stat-machines").textContent=o.machines;$("#stat-online").textContent=`${o.online} online`;$("#stat-containers").textContent=o.containers;$("#stat-running").textContent=`${o.running} running`;
  const online=agents.filter(a=>a.status==="online");$("#stat-load").textContent=online.length?(online.reduce((n,a)=>n+a.load1,0)/online.length).toFixed(2):"—";$("#stat-check").textContent="Now";
  $("#fleet-list").innerHTML=agents.length?agents.slice(0,6).map(a=>`<div class="fleet-row" data-id="${a.id}"><div class="machine-icon">▣</div><div><strong>${esc(a.name)}</strong><small>${esc(a.host)} · ${a.cpuCount||"—"} CPUs</small></div><span class="badge ${a.status}">${a.status}</span><div class="metric-mini"><b>${pct(a.memoryUsed,a.memoryTotal)}%</b><span>MEMORY</span></div><span>›</span></div>`).join(""):`<div class="empty"><strong>No machines connected yet</strong>Add your first server or VM over SSH.</div>`;
  const best=online[0];$("#pulse").className=best?"pulse-body":"pulse-empty";$("#pulse").innerHTML=best?`<p>Current snapshot from <strong>${esc(best.name)}</strong></p>${gauge("Memory",pct(best.memoryUsed,best.memoryTotal),`${fmtBytes(best.memoryUsed)} / ${fmtBytes(best.memoryTotal)}`)}${gauge("Disk",pct(best.diskUsed,best.diskTotal),`${fmtBytes(best.diskUsed)} / ${fmtBytes(best.diskTotal)}`)}${gauge("Load",Math.min(100,Math.round(best.load1/(best.cpuCount||1)*100)),best.load1.toFixed(2))}`:"Connect a machine to see its pulse.";
  $$(".fleet-row").forEach(el=>el.onclick=()=>showDetail(+el.dataset.id));
}
function gauge(label,value,caption){return `<div class="gauge"><div class="gauge-head"><span>${label}</span><strong>${caption}</strong></div><div class="bar"><i style="width:${value}%"></i></div></div>`}
function renderMachines(){
  $("#machine-table").innerHTML=`<div class="row head"><span>MACHINE</span><span>IP ADDRESS</span><span>TYPE</span><span>OPERATING SYSTEM</span><span>STATUS</span><span>CONTAINERS</span></div>`+(agents.length?agents.map(a=>`<div class="row" data-id="${a.id}"><strong>${esc(a.name)}</strong><span>${esc(a.host)}</span><span><b>${!a.virtualization||a.virtualization==="unknown"?"Detecting…":a.isVm?"Virtual machine":"Physical host"}</b><small>${esc(a.virtualization||"Detection pending")}</small></span><span class="os-cell" title="${esc(a.os||"Awaiting refresh")}">${esc(a.os||"Awaiting refresh")}</span><span class="badge ${a.status}">${a.status}</span><span>${a.runningCount} / ${a.containerCount}</span></div>`).join(""):`<div class="empty">No machines connected.</div>`);
  $$(".machine-table .row[data-id]").forEach(el=>el.onclick=()=>showDetail(+el.dataset.id));
}
function renderContainers(){
  const running=fleetContainers.filter(container=>container.state==="running").length;
  $("#container-summary").textContent=`${running} of ${fleetContainers.length} running`;
  $("#container-table").innerHTML=`<div class="container-fleet-row head"><span>APPLICATION</span><span>VERSION</span><span>MACHINE</span><span>UPTIME</span><span>PORTS</span><span>IMAGE</span><span>STATE</span></div>`+(fleetContainers.length?fleetContainers.map((container,index)=>`<button class="container-fleet-row container-click" data-container-index="${index}"><div><strong>${esc(container.name)}</strong><small>${esc(container.image.split(":")[0])}</small></div><code>${esc(container.version)}</code><span><strong>${esc(container.agentName)}</strong><small>${esc(container.agentHost)}</small></span><span class="uptime">${esc(container.uptime||"Unknown")}</span><span class="ports">${esc(compactPorts(container.ports))}</span><span class="badge pending update-status" data-update-index="${index}">Checking…</span><span class="badge ${container.state==="running"?"online":"offline"}">${esc(container.state)}</span></button>`).join(""):`<div class="empty"><strong>No containers discovered</strong>Refresh an online machine after Docker applications start.</div>`);
  $$("[data-container-index]").forEach(row=>row.onclick=()=>showContainerDetail(fleetContainers[+row.dataset.containerIndex]));
  if(!$("#containers-page").hidden)loadContainerUpdateStatuses();
}
async function loadContainerUpdateStatuses(){
  await Promise.allSettled(fleetContainers.map(async(container,index)=>{
    const key=`${container.agentId}:${container.id}:${container.image}`,cached=containerUpdateCache.get(key);
    const persistedFresh=container.imageCheckedAt&&Date.now()-new Date(container.imageCheckedAt).getTime()<86400000;
    let details=persistedFresh?container:null;
    if(persistedFresh&&cached&&Date.now()-cached.checkedAt<86400000)details=cached.details;
    if(!details){try{details=await api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}`);containerUpdateCache.set(key,{checkedAt:Date.now(),details});container.updateAvailable=details.updateAvailable;container.composeProject=details.composeProject;container.composeService=details.composeService;container.imageCheckedAt=new Date().toISOString()}catch{details={}}}
    const badge=$(`[data-update-index="${index}"]`);if(!badge)return;
    badge.className=`badge update-status ${details.updateAvailable===true?"pending":details.updateAvailable===false?"online":"neutral"}`;
    badge.textContent=details.updateAvailable===true&&details.composeProject&&details.composeService?"Update":details.updateAvailable===true?"Update available":details.updateAvailable===false?"Current":"Unavailable";
    if(details.updateAvailable===true&&details.composeProject&&details.composeService){
      badge.classList.add("update-action");badge.title=`Pull and recreate ${details.composeProject} / ${details.composeService}`;
      badge.onclick=async event=>{event.stopPropagation();if(!confirm(`Update ${container.name} and recreate its Docker Compose service?`))return;badge.onclick=null;badge.textContent="Updating…";try{await api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}/actions/update`,{method:"POST"});containerUpdateCache.delete(key);toast(`${container.name} updated`);await load()}catch(error){toast(error.message);containerUpdateCache.delete(key);loadContainerUpdateStatuses()}};
    }else if(details.updateAvailable===true)badge.title="Open details; automatic recreation is only available for Docker Compose services";
  }));
}
function showContainerDetail(container){
  $("#container-detail-content").innerHTML=`<div class="container-detail-head"><div><p class="eyebrow">DOCKER CONTAINER</p><h2>${esc(container.name)}</h2><p>${esc(container.agentName)} · ${esc(container.agentHost)}</p></div><button class="icon-btn" id="close-container-detail">×</button></div><div class="container-detail-grid"><div><small>STATE</small><span class="badge ${container.state==="running"?"online":"offline"}">${esc(container.state)}</span></div><div><small>UPTIME</small><strong>${esc(container.uptime||"Unknown")}</strong></div><div><small>VERSION</small><strong>${esc(container.version)}</strong></div><div><small>CREATED</small><strong>${esc(container.created||"Unknown")}</strong></div><div class="wide"><small>IMAGE</small><code>${esc(container.image)}</code></div><div class="wide"><small>CONTAINER ID</small><code>${esc(container.id)}</code></div><div class="wide"><small>PORTS</small><strong>${esc(compactPorts(container.ports))}</strong></div><div class="wide"><small>DOCKER STATUS</small><strong>${esc(container.status||container.state)}</strong></div></div><div class="dialog-actions"><button class="secondary" id="view-container-machine">View machine</button><button class="primary" id="done-container-detail">Done</button></div>`;
  const controls=document.createElement("div");controls.className="container-controls";controls.innerHTML=`<button class="control-button" id="refresh-container">Refresh</button><button class="control-button start-control" data-container-action="start" ${container.state==="running"?"disabled":""}>Start</button><button class="control-button stop-control" data-container-action="stop" ${container.state!=="running"?"disabled":""}>Stop</button><button class="control-button" data-container-action="restart">Restart</button>`;$(".dialog-actions").prepend(controls);
  $("#refresh-container").onclick=async()=>{const button=$("#refresh-container");button.disabled=true;button.textContent="Refreshing…";try{await api(`/api/agents/${container.agentId}/refresh`,{method:"POST"});containerUpdateCache.delete(`${container.agentId}:${container.id}:${container.image}`);$("#container-dialog").close();await load();const updated=fleetContainers.find(item=>item.id===container.id&&item.agentId===container.agentId);if(updated)showContainerDetail(updated);toast(`${container.name} refreshed`)}catch(error){toast(error.message);button.disabled=false;button.textContent="Refresh"}};
  $$("[data-container-action]").forEach(button=>button.onclick=async()=>{const action=button.dataset.containerAction;if((action==="stop"||action==="restart")&&!confirm(`${action[0].toUpperCase()+action.slice(1)} ${container.name}?`))return;$$("[data-container-action]").forEach(item=>item.disabled=true);button.textContent=`${action[0].toUpperCase()+action.slice(1)}ing…`;try{await api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}/actions/${action}`,{method:"POST"});$("#container-dialog").close();await load();const updated=fleetContainers.find(item=>item.id===container.id&&item.agentId===container.agentId);if(updated)showContainerDetail(updated);toast(`Container ${action==="stop"?"stopped":action+"ed"}`)}catch(error){toast(error.message);$("#container-dialog").close();showContainerDetail(container)}});
  const advanced=document.createElement("div");advanced.className="container-advanced";advanced.innerHTML=`<div class="advanced-loading">Checking image and runtime metadata…</div>`;$(".container-detail-grid").after(advanced);
  api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}`).then(details=>{
    const update=details.updateAvailable===true?`<span class="badge pending">New image available</span>`:details.updateAvailable===false?`<span class="badge online">Image is current</span>`:`<span class="badge pending">Registry check unavailable</span>`;
    const compose=details.composeProject?`<div class="wide"><small>DOCKER COMPOSE</small><strong>${esc(details.composeProject)} / ${esc(details.composeService||"service")}</strong></div>`:"";
    advanced.innerHTML=`<div class="advanced-head"><div><h3>Image & runtime</h3><p>Digest comparison checks the currently configured image tag.</p></div>${update}</div><div class="advanced-grid"><div><small>PLATFORM</small><strong>${esc(details.platform||"Unknown")}</strong></div><div><small>IMAGE SIZE</small><strong>${fmtBytes(details.imageSize)}</strong></div><div><small>RESTART POLICY</small><strong>${esc(details.restartPolicy||"none")}</strong></div><div><small>HEALTH CHECK</small><strong>${esc(details.health||"none")}</strong></div>${compose}<div class="wide"><small>LOCAL DIGEST</small><code>${esc(details.localDigest||details.imageId||"Unavailable")}</code></div><div class="wide"><small>REGISTRY DIGEST</small><code>${esc(details.registryDigest||"Unavailable")}</code></div><div class="wide"><small>NETWORKS</small><strong>${esc((details.networks||[]).join(", ")||"None")}</strong></div><div class="wide"><small>MOUNTS</small><strong>${esc((details.mounts||[]).join(", ")||"None")}</strong></div></div>`;
  }).catch(error=>advanced.innerHTML=`<div class="advanced-error">Additional metadata could not be loaded: ${esc(error.message)}</div>`);
  $("#close-container-detail").onclick=()=>$("#container-dialog").close();
  $("#done-container-detail").onclick=()=>$("#container-dialog").close();
  $("#view-container-machine").onclick=()=>{$("#container-dialog").close();showDetail(container.agentId)};
  $("#container-dialog").showModal();
}
async function showDetail(id,updateLocation=true){
  const a=agents.find(x=>x.id===id);if(!a)return;showPage("detail",false);if(updateLocation&&location.hash!==`#machine-${id}`)history.pushState(null,"",`#machine-${id}`);
  $("#detail-page").innerHTML=`<button class="back">← Back to machines</button><div class="detail-head"><div class="machine-icon">▣</div><div><h2>${esc(a.name)}</h2><p>${esc(a.username)}@${esc(a.host)}:${a.port} · ${esc(a.os||"Awaiting first refresh")}</p></div><span class="badge ${a.status}">${a.status}</span></div><section class="panel"><div class="detail-stats"><div><small>LOAD</small><strong>${a.load1.toFixed(2)}</strong></div><div><small>MEMORY</small><strong>${pct(a.memoryUsed,a.memoryTotal)}%</strong></div><div><small>DISK</small><strong>${pct(a.diskUsed,a.diskTotal)}%</strong></div><div><small>UPTIME</small><strong>${a.uptimeSeconds?Math.floor(a.uptimeSeconds/86400)+" days":"—"}</strong></div></div></section><section class="panel containers"><div class="panel-head"><div><h2>Docker applications</h2><p>Containers discovered on this machine</p></div><button class="secondary" id="refresh-one">↻ Refresh</button></div><div id="container-list"><div class="empty">Loading containers…</div></div></section>`;
  $(".back").onclick=()=>showPage("machines");$("#refresh-one").onclick=async()=>{try{await api(`/api/agents/${id}/refresh`,{method:"POST"});toast("Machine refreshed");await load();showDetail(id)}catch(e){toast(e.message)}};
  try{const cs=await api(`/api/agents/${id}/containers`);$("#container-list").innerHTML=cs.length?`<div class="container-row"><strong>NAME</strong><strong>IMAGE</strong><strong>STATE</strong><strong>PORTS</strong></div>`+cs.map(c=>`<div class="container-row"><strong>${esc(c.name)}</strong><span>${esc(c.image)}</span><span class="badge ${c.state==="running"?"online":"offline"}">${esc(c.state)}</span><span>${esc(c.ports||"—")}</span></div>`).join(""):`<div class="empty">No Docker containers discovered.</div>`}catch(e){toast(e.message)}
}
function showPage(page,updateLocation=true){
  ["overview","machines","containers","detail","settings"].forEach(p=>$(`#${p}-page`).hidden=p!==page);$$(".nav-item").forEach(n=>n.classList.toggle("active",n.dataset.page===page));
  $("#page-title").textContent=page==="overview"?"Good morning.":page==="machines"?"Your machines.":"Machine details.";
  $("#subtitle").textContent=page==="overview"?"Here’s what’s happening across your machines.":page==="machines"?"Every connected host, without an installed agent.":"A live operational snapshot over SSH.";
  if(page==="settings"){$("#page-title").textContent="Settings.";$("#subtitle").textContent="Manage appearance and reusable SSH credentials.";renderKeys()}
  if(page==="containers"){$("#page-title").textContent="Containers.";$("#subtitle").textContent="Docker applications across every connected machine.";renderContainers()}
  $("#add-machine").hidden=page==="settings"||page==="containers";
  if(updateLocation&&page!=="detail"&&location.hash!==`#${page}`)history.pushState(null,"",`#${page}`);
}
function routeFromLocation(){
  const route=location.hash.slice(1);
  const machine=route.match(/^machine-(\d+)$/);
  if(machine){showDetail(+machine[1],false);return}
  if(["overview","machines","containers","settings"].includes(route)){showPage(route,false);return}
  history.replaceState(null,"","#overview");showPage("overview",false);
}
function esc(s){const d=document.createElement("div");d.textContent=s??"";return d.innerHTML}
function setTheme(theme){document.documentElement.dataset.theme=theme;localStorage.setItem("secontrol-theme",theme);const input=$(`input[name="theme"][value="${theme}"]`);if(input)input.checked=true}
function renderKeys(){
  const select=$("#machine-key-select");
  const selectedKeyID=select.value;
  select.innerHTML=sshKeys.length?`<option value="">Choose a saved key…</option>`+sshKeys.map(k=>`<option value="${k.id}">${esc(k.name)}</option>`).join(""):`<option value="">No saved keys — add one in Settings</option>`;
  $("#key-list").innerHTML=sshKeys.length?sshKeys.map(k=>`<div class="key-row"><div><strong>${esc(k.name)}</strong><small>${esc((k.publicKey||"Public key unavailable").slice(0,54))}${k.publicKey?.length>54?"…":""} · Added ${new Date(k.createdAt).toLocaleDateString()}</small></div><button class="danger-button" data-delete-key="${k.id}">Remove</button></div>`).join(""):`<div class="key-empty">No SSH keys saved yet.</div>`;
  $$("[data-delete-key]").forEach(button=>button.onclick=async()=>{if(!confirm("Remove this saved SSH key? Existing machines will continue to work."))return;try{await api(`/api/ssh-keys/${button.dataset.deleteKey}`,{method:"DELETE"});sshKeys=sshKeys.filter(k=>k.id!==+button.dataset.deleteKey);renderKeys();toast("SSH key removed")}catch(e){toast(e.message)}});
  sshKeys.forEach(key=>{
    const button=$(`[data-delete-key="${key.id}"]`),meta=button?.closest(".key-row")?.querySelector("small"),usage=key.usageCount||0;
    if(meta)meta.append(` · ${usage} machine${usage===1?"":"s"}`);
    if(button&&usage>0){button.disabled=true;button.title="This key is assigned to a machine and cannot be removed"}
  });
  if(selectedKeyID&&sshKeys.some(key=>String(key.id)===selectedKeyID))select.value=selectedKeyID;
  select.onchange=()=>{showSetupCommand();invalidateConnectionTest()};
  if(select.value)showSetupCommand();else $("#key-setup").hidden=true;
}
function showSetupCommand(){
  const key=sshKeys.find(k=>k.id===+$("#machine-key-select").value),box=$("#key-setup");
  if(!key?.publicKey){box.hidden=true;return}
  const quoted=key.publicKey.replaceAll("'","'\\''");
  $("#setup-command").textContent=`install -d -m 700 "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys" && chmod 600 "$HOME/.ssh/authorized_keys"
grep -qxF '${quoted}' "$HOME/.ssh/authorized_keys" || \\
  printf '%s\\n' '${quoted}' >> "$HOME/.ssh/authorized_keys"`;
  box.hidden=false;
}
function machinePayload(){
  const data=Object.fromEntries(new FormData($("#machine-form")));
  data.port=+data.port;
  data.keyId=+data.keyId;
  return data;
}
function invalidateConnectionTest(){
  connectionVerified=false;
  $("#save-machine").disabled=true;
  $("#connection-result").hidden=true;
}
$$(".nav-item").forEach(n=>n.onclick=()=>showPage(n.dataset.page));$$("[data-page-link]").forEach(n=>n.onclick=()=>showPage(n.dataset.pageLink));
$("#add-machine").onclick=()=>{renderKeys();invalidateConnectionTest();$("#machine-dialog").showModal()};$$("[data-close]").forEach(n=>n.onclick=()=>$("#machine-dialog").close());
$$('input[name="theme"]').forEach(r=>r.onchange=()=>setTheme(r.value));
$("#show-key-form").onclick=()=>{$("#key-form").reset();$$(".import-key-field").forEach(el=>el.hidden=true);$(".generate-key-help").hidden=false;$("#key-error").textContent="";$("#key-dialog").showModal()};
function closeKeyDialog(){$("#key-form").reset();$("#key-error").textContent="";$("#key-dialog").close()}
$("#cancel-key").onclick=closeKeyDialog;$("#cancel-key-bottom").onclick=closeKeyDialog;
$$('input[name="keyMode"]').forEach(r=>r.onchange=()=>{$$(".import-key-field").forEach(el=>el.hidden=r.value!=="import");$(".generate-key-help").hidden=r.value!=="generate"});
$("#key-form").onsubmit=async e=>{e.preventDefault();const data=Object.fromEntries(new FormData(e.target)),generate=data.keyMode==="generate";delete data.keyMode;$("#key-error").textContent="";try{await api(generate?"/api/ssh-keys/generate":"/api/ssh-keys",{method:"POST",body:JSON.stringify(data)});closeKeyDialog();sshKeys=await api("/api/ssh-keys");renderKeys();toast(generate?"Ed25519 key generated securely":"SSH key saved securely")}catch(err){$("#key-error").textContent=err.message}};
$("#copy-setup").onclick=async()=>{await navigator.clipboard.writeText($("#setup-command").textContent);toast("Setup command copied")};
$$('input[name="authType"]').forEach(r=>r.onchange=()=>{$(".key-field").hidden=r.value!=="key";$(".password-field").hidden=r.value!=="password";invalidateConnectionTest()});
$("#machine-form").addEventListener("input",invalidateConnectionTest);
$("#test-connection").onclick=async()=>{const data=machinePayload(),button=$("#test-connection");$("#form-error").textContent="";$("#connection-result").hidden=true;if(data.authType==="key"&&!data.keyId){$("#form-error").textContent="Select an SSH key before testing.";return}button.disabled=true;button.textContent="Testing…";try{const result=await api("/api/agents/test",{method:"POST",body:JSON.stringify(data)});connectionVerified=true;$("#save-machine").disabled=false;$("#connection-result").textContent=`Connected to ${result.hostname||data.host}${result.system?` · ${result.system}`:""}`;$("#connection-result").hidden=false;toast("SSH connection successful")}catch(err){invalidateConnectionTest();$("#form-error").textContent=err.message}finally{button.disabled=false;button.textContent="Test connection"}};
$("#machine-form").onsubmit=async e=>{e.preventDefault();if(!connectionVerified){$("#form-error").textContent="Test the SSH connection before adding this machine.";return}const data=machinePayload();$("#form-error").textContent="";try{await api("/api/agents",{method:"POST",body:JSON.stringify(data)});$("#machine-dialog").close();e.target.reset();invalidateConnectionTest();toast("Machine added — running first check");await load()}catch(err){$("#form-error").textContent=err.message}};
$("#refresh-all").onclick=async()=>{await Promise.allSettled(agents.map(a=>api(`/api/agents/${a.id}/refresh`,{method:"POST"})));toast("Fleet refreshed");load()};
window.addEventListener("popstate",()=>{if(routeReady)routeFromLocation()});
setTheme(initialTheme);load();setInterval(load,30000);
