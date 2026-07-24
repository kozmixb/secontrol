const $=s=>document.querySelector(s), $$=s=>document.querySelectorAll(s);
const initialTheme=localStorage.getItem("secontrol-theme")||"dark";
document.documentElement.dataset.theme=initialTheme;
let agents=[],sshKeys=[],fleetContainers=[],fleetStorage=[],connectionVerified=false,routeReady=false;
const containerUpdateCache=new Map();
const fmtBytes=n=>{if(!n)return "—";const u=["B","KB","MB","GB","TB"];let i=0;while(n>=1024&&i<u.length-1){n/=1024;i++}return `${n.toFixed(i>2?1:0)} ${u[i]}`};
const pct=(used,total)=>total?Math.round(used/total*100):0;
const ago=value=>{if(!value)return "Never";const s=Math.max(0,(Date.now()-new Date(value))/1000);if(s<60)return "Just now";if(s<3600)return `${Math.floor(s/60)}m ago`;if(s<86400)return `${Math.floor(s/3600)}h ago`;return `${Math.floor(s/86400)}d ago`};
const fmtUptime=seconds=>{if(!seconds)return "—";const days=Math.floor(seconds/86400),hours=Math.floor(seconds%86400/3600);return days?`${days}d ${hours}h`:`${hours}h ${Math.floor(seconds%3600/60)}m`};
const compactPorts=value=>{const ports=[];for(const entry of (value||"").split(",")){let part=entry.trim();if(!part)continue;const udp=/\/udp$/i.test(part);part=part.replace(/\/(?:tcp|udp)$/i,"");if(part.includes("->")){let source=part.split("->")[0];source=source.startsWith("[")?source.slice(source.indexOf("]:")+2):source.split(":").pop();part=source}const label=part+(udp?"/udp":"");if(part&&!ports.includes(label))ports.push(label)}return ports.join(", ")||"—"};
const accessIcon=level=>level==="root"?`<span class="access-icon root-access" title="SSH user is root">#</span>`:level==="sudo"?`<span class="access-icon sudo-access" title="SSH user is a sudoer">◆</span>`:`<span class="access-icon regular-access" title="Regular SSH user">●</span>`;
const shortDigest=value=>value&&value.startsWith("sha256:")?value.slice(7,19):value||"Unavailable";
async function api(url,opts={}){const r=await fetch(url,{headers:{"Content-Type":"application/json"},...opts});if(!r.ok){const e=await r.json().catch(()=>({error:r.statusText}));throw Error(e.error)}return r.status===204?null:r.json()}
function toast(message){const t=$("#toast");t.textContent=message;t.classList.add("show");setTimeout(()=>t.classList.remove("show"),2600)}
async function load(){
  try{const [overview,list,keys,containers,storage]=await Promise.all([api("/api/overview"),api("/api/agents"),api("/api/ssh-keys"),api("/api/containers"),api("/api/storage")]);agents=list;sshKeys=keys;fleetContainers=containers;fleetStorage=storage;renderOverview(overview);renderMachines();renderContainers();renderStorage();renderKeys();if(!routeReady){routeReady=true;routeFromLocation()}}catch(e){toast(e.message)}
}
function renderOverview(o){
  $("#stat-machines").textContent=o.machines;$("#stat-online").textContent=`${o.online} online`;$("#stat-containers").textContent=o.containers;$("#stat-running").textContent=`${o.running} running`;
  $("#stat-storage").textContent=fmtBytes(o.storageAvailable);$("#stat-storage-total").textContent=`of ${fmtBytes(o.storageTotal)} total`;$("#stat-services").textContent=o.services;$("#stat-service-health").textContent=`${o.servicesRunning} running · ${o.servicesFailed} failed`;
  $("#fleet-list").innerHTML=agents.length?agents.slice(0,6).map(a=>`<div class="fleet-row" data-id="${a.id}"><div class="machine-icon">▣</div><div><strong>${esc(a.name)}</strong><small>${esc(a.host)} · ${a.cpuCount||"—"} CPUs</small></div><span class="badge ${a.status}">${a.status}</span><div class="metric-mini"><b>${pct(a.memoryUsed,a.memoryTotal)}%</b><span>MEMORY</span></div><span>›</span></div>`).join(""):`<div class="empty"><strong>No machines connected yet</strong>Add your first server or VM over SSH.</div>`;
  $("#pulse").className=agents.length?"capacity-body":"pulse-empty";$("#pulse").innerHTML=agents.length?`<div class="donut-grid">${donut("Machines",o.online,o.machines,"online","#2eb587")}${donut("Containers",o.running,o.containers,"running","#72b5e3")}${donut("Storage",o.storageUsed,o.storageTotal,"used","#a798e7",true)}</div><div class="fleet-service-health"><div><span class="health-dot healthy"></span><strong>${o.servicesRunning}</strong><small>Running services</small></div><div><span class="health-dot idle"></span><strong>${Math.max(0,o.services-o.servicesRunning-o.servicesFailed)}</strong><small>Stopped services</small></div><div><span class="health-dot failed"></span><strong>${o.servicesFailed}</strong><small>Failed services</small></div></div><button class="capacity-link" data-page-link="storage">View storage details →</button>`:"Connect a machine to see fleet capacity.";
  $$("[data-page-link]").forEach(n=>n.onclick=()=>showPage(n.dataset.pageLink));
  $$(".fleet-row").forEach(el=>el.onclick=()=>showDetail(+el.dataset.id));
}
function donut(label,value,total,state,colour,bytes=false){const percentage=pct(value,total);return `<div class="donut-card"><div class="donut" style="--value:${percentage};--donut-color:${colour}" role="img" aria-label="${esc(label)} ${percentage}% ${esc(state)}"><div><strong>${percentage}%</strong><span>${esc(state)}</span></div></div><strong>${esc(label)}</strong><small>${bytes?`${fmtBytes(value)} of ${fmtBytes(total)}`:`${value} of ${total}`}</small></div>`}
function gauge(label,value,caption){return `<div class="gauge"><div class="gauge-head"><span>${label}</span><strong>${caption}</strong></div><div class="bar"><i style="width:${value}%"></i></div></div>`}
function renderMachines(){
  $("#machine-table").innerHTML=`<div class="row head"><span>MACHINE</span><span>IP ADDRESS</span><span>TYPE</span><span>OPERATING SYSTEM</span><span>UPTIME</span><span>STATUS</span><span>CONTAINERS</span></div>`+(agents.length?agents.map(a=>`<div class="row" data-id="${a.id}"><strong class="machine-name">${accessIcon(a.accessLevel)}${esc(a.name)}</strong><span>${esc(a.host)}</span><span><b>${!a.virtualization||a.virtualization==="unknown"?"Detecting…":a.isVm?"Virtual machine":"Physical host"}</b><small>${esc(a.virtualization||"Detection pending")}</small></span><span class="os-cell" title="${esc(a.os||"Awaiting refresh")}">${esc(a.os||"Awaiting refresh")}</span><span class="uptime">${fmtUptime(a.uptimeSeconds)}</span><span class="badge ${a.status}">${a.status}</span><span>${a.runningCount} / ${a.containerCount}</span></div>`).join(""):`<div class="empty">No machines connected.</div>`);
  $$(".machine-table .row[data-id]").forEach(el=>el.onclick=()=>showDetail(+el.dataset.id));
}
function renderContainers(){
  const running=fleetContainers.filter(container=>container.state==="running").length;
  $("#container-summary").textContent=`${running} of ${fleetContainers.length} running`;
  renderContainerList($("#container-table"),fleetContainers,true);
}
function renderStorage(){
  const localStorage=fleetStorage.filter(volume=>!volume.isRemote),total=localStorage.reduce((sum,volume)=>sum+volume.totalBytes,0),used=localStorage.reduce((sum,volume)=>sum+volume.usedBytes,0);
  $("#storage-summary").textContent=`${fleetStorage.length} volumes · ${fmtBytes(used)} of ${fmtBytes(total)} local used`;
  $("#storage-list").innerHTML=agents.length?agents.map(agent=>{
    const volumes=fleetStorage.filter(volume=>volume.agentId===agent.id);
    return `<section class="storage-machine"><button class="storage-machine-head" data-storage-machine="${agent.id}"><div class="machine-icon">▤</div><div><strong>${esc(agent.name)}</strong><small>${esc(agent.host)} · ${volumes.length} volume${volumes.length===1?"":"s"}</small></div><span class="badge ${agent.status}">${agent.status}</span><span>›</span></button><div class="storage-columns"><span>VOLUME</span><span>MOUNT POINT</span><span>USED</span><span>CAPACITY</span></div>${volumes.length?volumes.map(volume=>{const usage=pct(volume.usedBytes,volume.totalBytes),level=usage>=90?"critical":usage>=75?"warning":"";return `<div class="storage-row"><div><strong>${esc(volume.filesystem)}</strong><small>${esc(volume.type||"filesystem")}${volume.isRemote?' · <span class="remote-volume">network share</span>':""}</small></div><code title="${esc(volume.mountPoint)}">${esc(volume.mountPoint)}</code><div class="storage-usage"><div><strong>${usage}%</strong><small>${fmtBytes(volume.usedBytes)} of ${fmtBytes(volume.totalBytes)}</small></div><div class="bar ${level}"><i style="width:${Math.min(100,usage)}%"></i></div></div><strong>${fmtBytes(volume.availableBytes)} <small>free${volume.isRemote?" · excluded from total":""}</small></strong></div>`}).join(""):`<div class="storage-empty">No storage data collected yet. Refresh this machine to collect its mounted filesystems.</div>`}</section>`;
  }).join(""):`<div class="empty">No machines connected.</div>`;
  $$("[data-storage-machine]").forEach(button=>button.onclick=()=>showDetail(+button.dataset.storageMachine));
}
function renderContainerList(target,containers,showMachine){
  const columns=`<span>APPLICATION</span><span>VERSION</span>${showMachine?"<span>MACHINE</span>":""}<span>UPTIME</span><span>IMAGE</span><span>STATE</span>`;
  target.innerHTML=`<div class="container-fleet-row head ${showMachine?"":"machine-containers"}">${columns}</div>`;
  if(!containers.length)target.append($("#container-empty-template").content.cloneNode(true));
  containers.forEach(container=>{
    const index=fleetContainers.findIndex(item=>item.agentId===container.agentId&&item.id===container.id),row=$("#container-row-template").content.firstElementChild.cloneNode(true);
    row.dataset.containerIndex=index;row.classList.toggle("machine-containers",!showMachine);
    row.querySelector("[data-container-name]").textContent=container.name;
    row.querySelector("[data-container-image-name]").textContent=container.image.split(":")[0];
    row.querySelector("[data-container-version]").textContent=container.version;
    const machine=row.querySelector("[data-container-machine]");
    if(showMachine){machine.querySelector("strong").innerHTML=accessIcon(container.agentAccess);machine.querySelector("strong").append(document.createTextNode(container.agentName));machine.querySelector("small").textContent=container.agentHost}else machine.remove();
    row.querySelector("[data-container-uptime]").textContent=container.uptime||"Unknown";
    const update=row.querySelector("[data-container-update]");update.dataset.updateIndex=index;
    const state=row.querySelector("[data-container-state]");state.classList.add(container.state==="running"?"online":"offline");state.textContent=container.state;
    target.append(row);
  });
  target.querySelectorAll("[data-container-index]").forEach(row=>row.onclick=()=>showContainerDetail(fleetContainers[+row.dataset.containerIndex]));
  loadContainerUpdateStatuses();
}
async function loadContainerUpdateStatuses(){
  await Promise.allSettled(fleetContainers.map(async(container,index)=>{
    const key=`${container.agentId}:${container.id}:${container.image}`,cached=containerUpdateCache.get(key);
    const persistedFresh=container.imageCheckedAt&&Date.now()-new Date(container.imageCheckedAt).getTime()<86400000;
    let details=persistedFresh?container:null;
    if(persistedFresh&&cached&&Date.now()-cached.checkedAt<86400000)details=cached.details;
    if(!details){try{details=await api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}`);containerUpdateCache.set(key,{checkedAt:Date.now(),details});container.updateAvailable=details.updateAvailable;container.composeProject=details.composeProject;container.composeService=details.composeService;container.imageCheckedAt=new Date().toISOString()}catch{details={}}}
    $$(`[data-update-index="${index}"]`).forEach(badge=>{
      badge.className=`badge update-status ${details.updateAvailable===true?"pending":details.updateAvailable===false?"online":"neutral"}`;
      badge.textContent=details.updateAvailable===true&&details.composeProject&&details.composeService?"Update":details.updateAvailable===true?"Update available":details.updateAvailable===false?"Current":"Unavailable";
      if(details.updateAvailable===true&&details.composeProject&&details.composeService){
        badge.classList.add("update-action");badge.title=`Pull and recreate ${details.composeProject} / ${details.composeService}`;
        badge.onclick=event=>{event.stopPropagation();showUpdateConfirmation(container,key,details)};
      }else if(details.updateAvailable===true)badge.title="Open details; automatic recreation is only available for Docker Compose services";
    });
  }));
}
async function showUpdateConfirmation(container,key,details){
  const dialog=$("#update-confirm-dialog");
  $("#update-confirm-title").textContent=`Update ${container.name}`;
  $("#update-confirm-subtitle").textContent=`${container.agentName} · ${details.composeProject} / ${details.composeService}`;
  $("#update-from-version").textContent=container.version||"Current";
  $("#update-to-version").textContent=container.version||"Latest";
  $("#update-from-digest").textContent="Loading digest…";
  $("#update-to-digest").textContent="Loading digest…";
  const confirmButton=$("#confirm-container-update");confirmButton.disabled=true;
  dialog.showModal();
  try{
    const metadata=details.localDigest&&details.registryDigest?details:await api(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}`);
    $("#update-confirm-subtitle").textContent=`${container.agentName} · ${metadata.composeProject||details.composeProject} / ${metadata.composeService||details.composeService}`;
    $("#update-from-digest").textContent=shortDigest(metadata.localDigest||metadata.imageId);
    $("#update-to-digest").textContent=shortDigest(metadata.registryDigest);
    confirmButton.disabled=!metadata.localDigest||!metadata.registryDigest;
    confirmButton.onclick=()=>{dialog.close();runContainerUpdate(container,key)};
  }catch(error){
    $("#update-from-digest").textContent="Unavailable";
    $("#update-to-digest").textContent="Unavailable";
    toast(error.message);
  }
}
async function runContainerUpdate(container,key){
  const dialog=$("#update-console-dialog"),output=$("#update-console-output"),state=$("#update-console-state"),close=$("#close-update-console");
  $("#update-console-title").textContent=`Updating ${container.name}`;$("#update-command").textContent="Preparing command…";output.textContent="Connecting to machine…\n";state.className="badge pending";state.textContent="Running";close.disabled=true;dialog.showModal();
  let buffer="",complete=false;
  const append=value=>{output.textContent+=value;output.scrollTop=output.scrollHeight};
  try{
    const response=await fetch(`/api/agents/${container.agentId}/containers/${encodeURIComponent(container.id)}/actions/update`,{method:"POST"});
    if(!response.ok){const error=await response.json().catch(()=>({error:response.statusText}));throw Error(error.error)}
    const reader=response.body.getReader(),decoder=new TextDecoder();
    while(true){const {value,done}=await reader.read();buffer+=decoder.decode(value||new Uint8Array(),{stream:!done});const lines=buffer.split("\n");buffer=lines.pop();for(const line of lines){if(!line)continue;const event=JSON.parse(line);if(event.type==="command"){$("#update-command").textContent=event.data;append(`\n$ ${event.data}\n\n`)}else if(event.type==="output")append(event.data);else if(event.type==="status")append(`\n${event.data}\n`);else if(event.type==="error")throw Error(event.data);else if(event.type==="complete"){append(`\n${event.data}\n`);complete=true}}if(done)break}
    if(!complete)throw Error("Update stream ended before completion");
    state.className="badge online";state.textContent="Complete";containerUpdateCache.delete(key);toast(`${container.name} updated`);await load();
  }catch(error){state.className="badge offline";state.textContent="Failed";append(`\nERROR: ${error.message}\n`);containerUpdateCache.delete(key);toast(error.message)}
  finally{close.disabled=false}
}
function showContainerDetail(container){
  $("#container-detail-content").innerHTML=`<div class="container-detail-head"><div><p class="eyebrow">DOCKER CONTAINER</p><h2>${esc(container.name)}</h2><p>${esc(container.agentName)} · ${esc(container.agentHost)}</p></div><button class="icon-btn" id="close-container-detail">×</button></div><div class="container-detail-grid"><div><small>STATE</small><span class="badge ${container.state==="running"?"online":"offline"}">${esc(container.state)}</span></div><div><small>UPTIME</small><strong>${esc(container.uptime||"Unknown")}</strong></div><div><small>VERSION</small><strong>${esc(container.version)}</strong></div><div><small>CREATED</small><strong>${esc(container.created||"Unknown")}</strong></div><div class="wide"><small>IMAGE</small><code>${esc(container.image)}</code></div><div class="wide"><small>PORTS</small><strong>${esc(compactPorts(container.ports))}</strong></div></div><div class="dialog-actions"><button class="secondary" id="view-container-machine">View machine</button><button class="primary" id="done-container-detail">Done</button></div>`;
  const controls=document.createElement("div");controls.className="container-controls";controls.innerHTML=`<button class="control-button" id="refresh-container">Refresh</button><button class="control-button start-control" data-container-action="start" ${container.state==="running"?"disabled":""}>Start</button><button class="control-button stop-control" data-container-action="stop" ${container.state!=="running"?"disabled":""}>Stop</button><button class="control-button" data-container-action="restart">Restart</button>`;$("#container-detail-content .dialog-actions").prepend(controls);
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
  $("#detail-page").innerHTML=`<button class="back">← Back to machines</button><div class="detail-head"><div class="machine-icon">▣</div><div><h2>${esc(a.name)}</h2><p>${esc(a.username)}@${esc(a.host)}:${a.port} · ${esc(a.os||"Awaiting first refresh")}</p></div><span class="badge ${a.status}">${a.status}</span></div><section class="panel"><div class="detail-stats"><div><small>LOAD</small><strong>${a.load1.toFixed(2)}</strong></div><div><small>MEMORY</small><strong>${pct(a.memoryUsed,a.memoryTotal)}%</strong></div><div><small>DISK</small><strong>${pct(a.diskUsed,a.diskTotal)}%</strong></div><div><small>UPTIME</small><strong>${a.uptimeSeconds?Math.floor(a.uptimeSeconds/86400)+" days":"—"}</strong></div></div></section><div class="machine-system-grid"><section class="panel system-panel"><div class="panel-head"><div><h2>Networks</h2><p>Interfaces and assigned addresses</p></div></div><div id="machine-networks"><div class="empty">Loading networks…</div></div></section><section class="panel system-panel"><div class="panel-head"><div><h2>Services</h2><p>System service health and state</p></div><div id="service-summary"></div></div><div id="machine-services"><div class="empty">Loading services…</div></div></section></div><section class="panel containers"><div class="panel-head"><div><h2>Docker applications</h2><p>Containers discovered on this machine</p></div><button class="secondary" id="refresh-one">↻ Refresh</button></div><div id="container-list"><div class="empty">Loading containers…</div></div></section>`;
  $(".back").onclick=()=>showPage("machines");$("#refresh-one").onclick=async()=>{try{await api(`/api/agents/${id}/refresh`,{method:"POST"});toast("Machine refreshed");await load();showDetail(id)}catch(e){toast(e.message)}};
  renderContainerList($("#container-list"),fleetContainers.filter(container=>container.agentId===id),false);
  try{renderMachineSystem(await api(`/api/agents/${id}/system`))}catch(e){toast(e.message)}
}
function renderMachineSystem(system){
  $("#machine-networks").innerHTML=system.networks.length?system.networks.map(network=>`<div class="network-row"><div><strong>${esc(network.name)}</strong><small>${esc(network.mac||"No hardware address")}</small></div><div>${(network.addresses||[]).map(address=>`<code>${esc(address)}</code>`).join("")||"<small>No assigned address</small>"}</div><span class="badge ${network.state==="up"?"online":"neutral"}">${esc(network.state||"unknown")}</span></div>`).join(""):`<div class="empty">No network interfaces collected.</div>`;
  const running=system.services.filter(service=>service.activeState==="active").length,failed=system.services.filter(service=>service.activeState==="failed").length,stopped=system.services.length-running-failed;
  $("#service-summary").innerHTML=`<span class="service-count running">${running} running</span><span class="service-count stopped">${stopped} stopped</span>${failed?`<span class="service-count failed">${failed} failed</span>`:""}`;
  $("#machine-services").innerHTML=system.services.length?system.services.map(service=>{const state=service.activeState==="failed"?"failed":service.activeState==="active"?"running":"stopped";return `<div class="service-row"><div><strong>${esc(service.name.replace(/\.service$/,""))}</strong><small>${esc(service.description||service.subState)}</small></div><span class="badge ${state==="running"?"online":state==="failed"?"offline":"neutral"}">${state}</span></div>`}).join(""):`<div class="empty">No systemd services collected.</div>`;
}
function showPage(page,updateLocation=true){
  ["overview","machines","containers","storage","detail","settings"].forEach(p=>$(`#${p}-page`).hidden=p!==page);$$(".nav-item").forEach(n=>n.classList.toggle("active",n.dataset.page===page));
  $("#page-title").textContent=page==="overview"?"Good morning.":page==="machines"?"Your machines.":"Machine details.";
  $("#subtitle").textContent=page==="overview"?"Here’s what’s happening across your machines.":page==="machines"?"Every connected host, without an installed agent.":"A live operational snapshot over SSH.";
  if(page==="settings"){$("#page-title").textContent="Settings.";$("#subtitle").textContent="Manage appearance and reusable SSH credentials.";renderKeys()}
  if(page==="containers"){$("#page-title").textContent="Containers.";$("#subtitle").textContent="Docker applications across every connected machine.";renderContainers()}
  if(page==="storage"){$("#page-title").textContent="Storage.";$("#subtitle").textContent="Filesystem capacity across your connected machines.";renderStorage()}
  $("#add-machine").hidden=page!=="machines";
  if(updateLocation&&page!=="detail"&&location.hash!==`#${page}`)history.pushState(null,"",`#${page}`);
}
function routeFromLocation(){
  const route=location.hash.slice(1);
  const machine=route.match(/^machine-(\d+)$/);
  if(machine){showDetail(+machine[1],false);return}
  if(["overview","machines","containers","storage","settings"].includes(route)){showPage(route,false);return}
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
function resetMachineDialog(){
  $("#machine-form").reset();
  $(".key-field").hidden=false;
  $(".password-field").hidden=true;
  $("#key-setup").hidden=true;
  $("#setup-command").textContent="";
  $("#form-error").textContent="";
  $("#connection-result").textContent="";
  $("#test-connection").disabled=false;
  $("#test-connection").textContent="Test connection";
  invalidateConnectionTest();
}
function closeMachineDialog(){resetMachineDialog();$("#machine-dialog").close()}
$$(".nav-item").forEach(n=>n.onclick=()=>showPage(n.dataset.page));$$("[data-page-link]").forEach(n=>n.onclick=()=>showPage(n.dataset.pageLink));
$("#add-machine").onclick=()=>{resetMachineDialog();renderKeys();$("#machine-dialog").showModal()};$$("[data-close]").forEach(n=>n.onclick=closeMachineDialog);
$("#machine-dialog").addEventListener("close",resetMachineDialog);
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
$("#machine-form").onsubmit=async e=>{e.preventDefault();if(!connectionVerified){$("#form-error").textContent="Test the SSH connection before adding this machine.";return}const data=machinePayload();$("#form-error").textContent="";try{await api("/api/agents",{method:"POST",body:JSON.stringify(data)});closeMachineDialog();toast("Machine added — running first check");await load()}catch(err){$("#form-error").textContent=err.message}};
$("#refresh-all").onclick=async()=>{await Promise.allSettled(agents.map(a=>api(`/api/agents/${a.id}/refresh`,{method:"POST"})));toast("Fleet refreshed");load()};
$("#close-update-confirm").onclick=()=>$("#update-confirm-dialog").close();
$("#cancel-container-update").onclick=()=>$("#update-confirm-dialog").close();
$("#close-update-console").onclick=()=>$("#update-console-dialog").close();
$$("dialog").forEach(dialog=>dialog.addEventListener("click",event=>{
  if(event.target!==dialog)return;
  const bounds=dialog.getBoundingClientRect(),outside=event.clientX<bounds.left||event.clientX>bounds.right||event.clientY<bounds.top||event.clientY>bounds.bottom;
  if(outside)dialog.close();
}));
window.addEventListener("popstate",()=>{if(routeReady)routeFromLocation()});
setTheme(initialTheme);load();setInterval(load,30000);
