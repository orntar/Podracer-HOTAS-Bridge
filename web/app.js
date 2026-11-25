'use strict';
const $ = id => document.getElementById(id);
let token = location.hash.slice(1) || sessionStorage.getItem('bridge-token') || '';
if (token) sessionStorage.setItem('bridge-token', token);
history.replaceState(null, '', location.pathname);
let config, state, meta, deviceList = [], dirty = false, busy = false, online = true;
let captured = {left: {}, right: {}}, profileNames = [], demoButtons = {};
const clone = x => JSON.parse(JSON.stringify(x));
async function api(path, data) {
  const opts = {headers: {'X-Bridge-Token': token}};
  if (data !== undefined) {opts.method = 'POST'; opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(data);}
  const r = await fetch('/api/' + path, opts);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}
function showError(e) {$('error').textContent = e.message || String(e); $('error').hidden = false;}
async function task(fn) {
  if (busy) return;
  busy = true; $('error').hidden = true;
  try {await fn();} catch(e) {showError(e);} finally {busy = false;}
}
function markDirty() {dirty = true; $('dirty').textContent = 'Unsaved changes · apply to update the engine.'; $('dirty').className = 'changed'; updateStart(); drawCurve();}
function markClean() {dirty = false; $('dirty').textContent = 'Settings applied · save a profile to keep them.'; $('dirty').className = ''; updateStart();}
function updateStart() {if (!state || !config) return; $('start').disabled = !online || state.demo || !state.device || state.active || dirty || !config.left.calibrated || !config.right.calibrated; $('stop').disabled = !online || !state.active;}
function switchTab(id) {document.querySelectorAll('.panel').forEach(p => p.hidden = p.id !== id); document.querySelectorAll('[data-tab]').forEach(b => b.classList.toggle('selected', b.dataset.tab === id)); if(id === 'tuning') drawCurve();}
document.querySelectorAll('[data-tab]').forEach(b => b.addEventListener('click', () => switchTab(b.dataset.tab)));
function options(select, entries, value) {select.replaceChildren(); for (const [val,label] of entries) {const o=document.createElement('option');o.value=val;o.textContent=label;select.append(o);}if(value !== undefined)select.value=value;}
async function refreshDevices() {const r=await api('devices');deviceList=r.devices;options($('devices'),[['','Select a controller'],...r.devices.map(d=>[d.path,`${d.name} · ${d.path}`])],state?.device?.path || ''); if(!r.devices.length) $('device-detail').textContent=r.warnings.length?'No readable joystick found. Open Device access & help to check permissions.':'No joystick found. Plug in the rig and click Refresh.';}
function renderConfig() {
  if (!config) return;
  captured={left:{idle:config.left.calibrated,full:config.left.calibrated},right:{idle:config.right.calibrated,full:config.right.calibrated}};
  const axes = state?.device?.axes || [{code:0,name:'X'},{code:1,name:'Y'}];
  $('calibration-fields').replaceChildren();
  for (const side of ['left','right']) {
    const div=document.createElement('div');div.className='cal-card';
    const heading=document.createElement('h4');heading.textContent=side==='left'?'Left handle':'Right handle';div.append(heading);
    const label=document.createElement('label');label.textContent='Input axis';
    const select=document.createElement('select');select.id=side+'-axis';options(select,axes.map(a=>[String(a.code),`${a.name} (axis ${a.code})`]),String(config[side].code));label.append(select);div.append(label);
    select.addEventListener('change',()=>{config[side].code=Number(select.value);config[side].calibrated=false;captured[side]={};updateCalibration();markDirty();});
    for (const endpoint of ['idle','full']) {
      const row=document.createElement('div');row.className='capture-row';const b=document.createElement('button');b.textContent=endpoint==='idle'?'Capture idle · fully back':'Capture full · fully forward';b.id=`${side}-${endpoint}-capture`;const out=document.createElement('output');out.id=`${side}-${endpoint}`;row.append(b,out);div.append(row);
      b.addEventListener('click',()=>task(async()=>{
        if(state.active) await api('stop',{});
        const fresh=await api('state');const value=fresh.raw[config[side].code];
        if(value===undefined||(!fresh.device&&!fresh.demo))throw new Error('Connect your controller before capturing travel.');
        config[side][endpoint]=value;captured[side][endpoint]=true;
        config[side].calibrated=!!(captured[side].idle&&captured[side].full&&Math.abs(config[side].full-config[side].idle)>=10);
        updateCalibration();markDirty();
      }));
    }
    const status=document.createElement('span');status.id=side+'-cal-status';status.className='cal-status';div.append(status);$('calibration-fields').append(div);
  }
  updateCalibration();renderTuning();renderMappings();drawCurve();updateStart();
}
function updateCalibration() {
  for(const side of ['left','right']){for(const endpoint of ['idle','full'])$(`${side}-${endpoint}`).textContent=captured[side][endpoint]?String(Math.round(config[side][endpoint])):'Not captured';const s=$(side+'-cal-status');s.textContent=config[side].calibrated?'✓ Both stops captured':captured[side].idle&&captured[side].full?'Stops are too close. Move the handle through its full travel.':'Capture both stops to enable output.';s.classList.toggle('ok',config[side].calibrated);}
}
const sliders=[
 ['steering_gain','Steering sensitivity',.1,4,.05,'×','Higher reaches full steering with a smaller handle difference.'],
 ['steering_curve','Steering response curve',.25,4,.05,'×','1.00 is linear; higher is gentler near the center.'],
 ['steering_deadzone','Steering center deadzone',0,.4,.005,'%','Ignore small differences between the two handles.'],
 ['throttle_curve','Throttle response curve',.25,4,.05,'×','Shapes acceleration when both handles move together.'],
 ['throttle_limit','Maximum throttle',0,1,.01,'%','Limit the maximum accelerator output.'],
 ['preserve_thrust','Preserve thrust through turns',0,1,.01,'%','0% = average handle; 100% = higher handle.'],
 ['left.deadzone','Left handle idle deadzone',0,.25,.005,'%','Ignore a little movement at the back stop.'],
 ['right.deadzone','Right handle idle deadzone',0,.25,.005,'%','Ignore a little movement at the back stop.'],
 ['left.curve','Left handle response curve',.25,4,.05,'×','Applied before the steering and throttle mixer.'],
 ['right.curve','Right handle response curve',.25,4,.05,'×','Applied before the steering and throttle mixer.']
];
function getValue(path){return path.split('.').reduce((v,k)=>v[k],config);}
function setValue(path,value){const keys=path.split('.');if(keys.length===1)config[path]=value;else config[keys[0]][keys[1]]=value;}
function formatted(v,unit){return unit==='%'?Math.round(v*100)+'%':v.toFixed(2)+'×';}
function renderTuning(){
  $('tuning-fields').replaceChildren();
  for(const [key,title,min,max,step,unit,hint] of sliders){
    const row=document.createElement('div');row.className='slider-row';const head=document.createElement('div');head.className='slider-head';const label=document.createElement('label');label.textContent=title;const id='tune-'+key.replace('.','-');label.htmlFor=id;const out=document.createElement('output');out.textContent=formatted(getValue(key),unit);const input=document.createElement('input');Object.assign(input,{type:'range',min,max,step,value:getValue(key),id});const small=document.createElement('small');small.textContent=hint;head.append(label,out);row.append(head,input,small);$('tuning-fields').append(row);
    input.addEventListener('input',()=>{setValue(key,Number(input.value));out.textContent=formatted(Number(input.value),unit);markDirty();});
  }
  $('invert-steering').checked=config.invert_steering;$('brake-cuts').checked=config.brake_cuts_throttle;
}
$('invert-steering').addEventListener('change',e=>{config.invert_steering=e.target.checked;markDirty();});
$('brake-cuts').addEventListener('change',e=>{config.brake_cuts_throttle=e.target.checked;markDirty();});
function renderMappings(){
  $('mapping-rows').replaceChildren();const count=Math.max(20,state?.device?.buttons||0);
  for(let n=1;n<=Math.min(count,128);n++){
    const tr=document.createElement('tr');tr.id='button-'+n;const num=document.createElement('td');num.textContent=String(n).padStart(2,'0');const name=document.createElement('td');name.textContent=meta.labels[n]||'Spare / custom control';const td=document.createElement('td');const select=document.createElement('select');select.setAttribute('aria-label','Output for button '+n);options(select,meta.actions.map(a=>[a.id,a.label]),config.mappings[n]||'none');select.addEventListener('change',()=>{config.mappings[n]=select.value;markDirty();});td.append(select);tr.append(num,name,td);$('mapping-rows').append(tr);
  }
}
function drawCurve(){
  if(!config)return;const canvas=$('curve'),ctx=canvas.getContext('2d'),w=canvas.width,h=canvas.height,p=42;
  ctx.clearRect(0,0,w,h);ctx.strokeStyle='#263649';ctx.lineWidth=1;ctx.font='12px system-ui';ctx.fillStyle='#8aa0ba';
  for(let i=0;i<=4;i++){const x=p+(w-2*p)*i/4,y=h-p-(h-2*p)*i/4;ctx.beginPath();ctx.moveTo(x,p);ctx.lineTo(x,h-p);ctx.stroke();ctx.beginPath();ctx.moveTo(p,y);ctx.lineTo(w-p,y);ctx.stroke();ctx.fillText(String(i*25),x-8,h-p+20);ctx.fillText(String(i*25),8,y+4);}
  ctx.fillText('Input %',w/2-20,h-7);ctx.fillText('Output %',p,20);
  function curve(color,fn){ctx.strokeStyle=color;ctx.lineWidth=2.5;ctx.beginPath();for(let i=0;i<=100;i++){const x=i/100,y=fn(x);if(i===0)ctx.moveTo(p+x*(w-2*p),h-p-y*(h-2*p));else ctx.lineTo(p+x*(w-2*p),h-p-y*(h-2*p));}ctx.stroke();}
  curve('#65e3c9',x=>Math.pow(x,config.throttle_curve)*config.throttle_limit);
  curve('#f1b86d',x=>Math.min(1,Math.pow(Math.max(0,(x-config.steering_deadzone)/(1-config.steering_deadzone)),config.steering_curve)*config.steering_gain));
}
function renderState(s){
  state=s;online=true;$('notice').textContent=s.message;$('mode').textContent=s.demo?'DEMO · NO OUTPUT':s.active?(s.armed?'OUTPUT ACTIVE':'WAITING FOR IDLE'):'OUTPUT OFF';$('mode').className='badge'+(s.active?(s.armed?' on':' wait'):'');
  $('demo-controls').hidden=!s.demo;
  for(const side of ['left','right']){$(side+'-value').textContent=Math.round(s.preview[side]*100)+'%';$(side+'-meter').value=s.preview[side]*100;$(side+'-raw').textContent='Raw '+(s.raw[s.config[side].code]??'—');}
  const steer=s.preview.steering;$('steer-value').textContent=Math.abs(steer)<.005?'CENTER':Math.round(Math.abs(steer)*100)+'% '+(steer<0?'LEFT':'RIGHT');$('steer-dot').style.left=(50+steer*46)+'%';$('throttle-value').textContent=Math.round(s.preview.throttle*100)+'%';$('brake-value').textContent=Math.round(s.preview.brake*100)+'%';$('throttle-meter').value=s.preview.throttle*100;
  $('monitor-note').textContent=s.active&&s.armed?'Live output · keyboard actions go to the focused window.':'Preview from applied settings · no input is being sent to games.';
  document.querySelectorAll('#mapping-rows tr').forEach(tr=>tr.classList.toggle('pressed',!!s.buttons[Number(tr.id.slice(7))]));
  if(s.device)$('device-detail').textContent=`${s.device.name} · ${s.device.axes.length} axes · ${s.device.buttons} buttons · USB ${s.device.vendor.toString(16).padStart(4,'0')}:${s.device.product.toString(16).padStart(4,'0')}`;
  else if(!s.demo)$('device-detail').textContent='No controller connected to the engine. Select the rig and click Connect.';
  updateStart();
}
async function poll(){try{renderState(await api('state'));}catch(e){online=false;$('mode').textContent='APP UNREACHABLE';$('mode').className='badge wait';$('notice').textContent='Connection to the bridge was lost. Output state is unknown; reopen the app or disconnect the rig.';updateStart();}finally{setTimeout(poll,200);}}
async function syncConfig(){state=await api('state');config=clone(state.config);renderConfig();markClean();renderState(state);}
async function profiles(){profileNames=await api('profiles');options($('profile-list'),profileNames.map(n=>[n,n]));}
async function apply(){await api('config',config);await syncConfig();}
$('apply').addEventListener('click',()=>task(apply));
$('refresh').addEventListener('click',()=>task(refreshDevices));
$('connect').addEventListener('click',()=>task(async()=>{if(!$('devices').value)throw new Error('Select your controller first.');if(dirty&&!confirm('Connecting will replace unsaved edits with the engine settings. Continue?'))return;await api('select',{path:$('devices').value});await syncConfig();}));
$('disconnect').addEventListener('click',()=>task(async()=>{await api('disconnect',{});renderState(await api('state'));}));
$('start').addEventListener('click',()=>task(async()=>{await api('start',{});renderState(await api('state'));}));
$('stop').addEventListener('click',()=>task(async()=>{await api('stop',{});renderState(await api('state'));}));
$('save').addEventListener('click',()=>task(async()=>{const name=$('profile-name').value.trim();if(!/^[A-Za-z0-9][A-Za-z0-9 _-]{0,47}$/.test(name))throw new Error('Use 1–48 letters, numbers, spaces, hyphens or underscores for the profile name.');if(profileNames.includes(name)&&!confirm(`Replace saved profile “${name}”?`))return;await apply();await api('save',{name,config});await profiles();$('dirty').textContent='Profile saved: '+name;$('profile-list').value=name;}));
$('load').addEventListener('click',()=>task(async()=>{const name=$('profile-list').value;if(!name)throw new Error('No saved profile selected.');if(dirty&&!confirm('Discard unsaved edits and load this profile?'))return;await api('load',{name});await syncConfig();$('profile-name').value=name;await refreshDevices();}));
$('export').addEventListener('click',()=>{const a=document.createElement('a'),url=URL.createObjectURL(new Blob([JSON.stringify(config,null,2)],{type:'application/json'}));a.href=url;a.download=($('profile-name').value.trim()||'Podracer')+'.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);});
$('import').addEventListener('change',()=>task(async()=>{const file=$('import').files[0];if(!file)return;if(file.size>65536)throw new Error('Profile is too large.');const c=JSON.parse(await file.text());if(dirty&&!confirm('Discard unsaved edits and import this profile?'))return;await api('disconnect',{});await api('config',c);await syncConfig();$('profile-name').value=file.name.replace(/\.json$/i,'').slice(0,48);$('dirty').textContent='Imported · save a profile to keep these settings.';$('import').value='';}));
$('quit').addEventListener('click',()=>task(async()=>{if(dirty&&!confirm('Quit and discard unsaved changes?'))return;await api('quit',{});$('notice').textContent='Bridge stopped. You can close this tab.';window.close();}));
function shellQuote(s){return "'"+s.replaceAll("'","'\\''")+"'";}
function setupCommand(){const id=$('usb-device').value;if(!id)return '';return `sudo env APPIMAGE_EXTRACT_AND_RUN=1 ${shellQuote(meta.access.executable)} --install-access ${shellQuote(id)}`;}
$('usb-device').addEventListener('change',()=>{$('setup-command').textContent=setupCommand()||'Choose the controller to generate its setup command.';});
$('copy-setup').addEventListener('click',()=>task(async()=>{const cmd=setupCommand();if(!cmd)throw new Error('Select the Arduino USB device first.');await navigator.clipboard.writeText(cmd);$('copy-setup').textContent='Copied';setTimeout(()=>$('copy-setup').textContent='Copy setup command',1500);}));
async function demo(){try{await api('demo',{left:Number($('demo-left').value),right:Number($('demo-right').value),buttons:demoButtons});}catch(e){showError(e);}}
$('demo-left').addEventListener('input',demo);$('demo-right').addEventListener('input',demo);
$('demo-brake').addEventListener('pointerdown',e=>{e.target.setPointerCapture(e.pointerId);demoButtons[1]=true;demo();});
for(const event of ['pointerup','pointercancel','lostpointercapture'])$('demo-brake').addEventListener(event,()=>{demoButtons[1]=false;demo();});
window.addEventListener('beforeunload',e=>{if(dirty){e.preventDefault();e.returnValue='';}});
(async()=>{try{meta=await api('meta');$('version').textContent=meta.version;$('profile-path').textContent='Saved locally: '+meta.profile_dir;$('access-status').textContent=meta.access.uinput_access?'Virtual controller access is available. If the rig is listed and connects, no setup is needed.':'Virtual controller access is not yet available. The one-time setup below may be needed.';options($('usb-device'),[['','Select the Arduino / Pro Micro USB device'],...meta.access.usb_devices.map(d=>[d.id,`${d.name} (${d.id})`])]);await syncConfig();await refreshDevices();await profiles();poll();}catch(e){showError(e);$('notice').textContent='Unable to connect. Launch the app again and use its dashboard link.';}})();
