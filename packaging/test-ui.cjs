// Run against an explicitly launched --demo instance. Never exercises hardware.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {spawn}=require('node:child_process');
(async()=>{
 const child=spawn(path.resolve('build/PodracerBridge-demo.exe'),['--demo','--no-browser','--config-dir','./build/test-profiles'],{windowsHide:true,stdio:['ignore','pipe','pipe']});
 const url=await new Promise((resolve,reject)=>{let text='';child.stdout.on('data',d=>{text+=d;const m=text.match(/Dashboard: (http[^\s]+)/);if(m)resolve(m[1]);});child.on('error',reject);child.on('exit',c=>reject(new Error('demo exited: '+c)));});
 const browser=await chromium.launch({headless:true,channel:'chrome'});
 try{
  const page=await browser.newPage({viewport:{width:1360,height:1100}}),errors=[];
  page.on('pageerror',e=>errors.push(e.message));page.on('dialog',d=>d.accept());
  await page.goto(url);
  await page.getByText('DEMO · NO OUTPUT',{exact:true}).waitFor();
  assert.equal(await page.locator('#start').isDisabled(),true);
  async function range(id,value){await page.locator('#'+id).evaluate((el,v)=>{el.value=v;el.dispatchEvent(new Event('input',{bubbles:true}));},String(value));await page.waitForTimeout(350);}
  for(const side of ['left','right']){
   await range('demo-'+side,0);await page.locator('#'+side+'-idle-capture').click();await page.waitForTimeout(150);
   await range('demo-'+side,1);await page.locator('#'+side+'-full-capture').click();await page.waitForTimeout(150);
   assert.match(await page.locator('#'+side+'-cal-status').textContent(),/Both stops captured/);
  }
  await page.locator('#apply').click();await page.waitForTimeout(300);
  await range('demo-left',.8);await range('demo-right',.4);
  assert.notEqual(await page.locator('#steer-value').textContent(),'CENTER');
  await page.locator('[data-tab="tuning"]').click();await range('tune-steering_gain',1.5);await range('tune-throttle_limit',.8);
  await page.locator('#apply').click();await page.waitForTimeout(300);
  assert.equal(await page.locator('#tune-steering_gain').inputValue(),'1.5');
  await page.screenshot({path:'build/dashboard.png',fullPage:true});
  await page.locator('[data-tab="buttons"]').click();
  await page.locator('#button-13 select').selectOption('key:ENTER');
  await page.locator('[data-tab="profiles"]').click();await page.locator('#profile-name').fill('UI Test');await page.locator('#save').click();
  await page.waitForFunction(()=>document.getElementById('dirty').textContent.includes('Profile saved: UI Test'));
  const saved=JSON.parse(fs.readFileSync('build/test-profiles/UI Test.json','utf8'));
  assert.equal(saved.mappings['13'],'key:ENTER');assert.equal(saved.steering_gain,1.5);assert.equal(saved.left.calibrated,true);
  await page.locator('#load').click();await page.waitForTimeout(300);
  await page.locator('[data-tab="buttons"]').click();assert.equal(await page.locator('#button-13 select').inputValue(),'key:ENTER');
  await page.locator('[data-tab="profiles"]').click();const downloadPromise=page.waitForEvent('download');await page.locator('#export').click();const download=await downloadPromise;await download.saveAs('build/exported-profile.json');
  assert.equal(JSON.parse(fs.readFileSync('build/exported-profile.json','utf8')).mappings['13'],'key:ENTER');
  await page.locator('#import').setInputFiles(path.resolve('build/exported-profile.json'));await page.waitForTimeout(300);
  await page.locator('[data-tab="access"]').click();await page.getByRole('heading',{name:'Bazzite setup',exact:true}).waitFor();
  await page.setViewportSize({width:640,height:1000});await page.locator('[data-tab="calibration"]').click();await page.screenshot({path:'build/dashboard-narrow.png',fullPage:true});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'page should not overflow horizontally');
  assert.equal(await page.locator('#error').isHidden(),true);
  assert.deepEqual(errors,[]);console.log('UI PASS: calibration, simulated input, tuning, mappings, save/load, export/import, help, narrow layout; no browser errors.');
 }finally{await browser.close();const u=new URL(url);await fetch(u.origin+'/api/quit',{method:'POST',headers:{'X-Bridge-Token':u.hash.slice(1)}}).catch(()=>child.kill());}
})().catch(e=>{console.error(e);process.exit(1);});
