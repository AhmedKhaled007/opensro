import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {launchProbeBrowser} from '../../../../scripts/lib/probeBrowser.mjs';
import {CLIENT_NEXT_BASE_URL} from '../../../../scripts/lib/probeEndpoints.mjs';
test('GM console uses production input, authority gates and reply channels',{timeout:90000},async()=>{
 const {browser,page}=await launchProbeBrowser();
 try{
 await page.setViewportSize({width:1200,height:900});await page.goto(CLIENT_NEXT_BASE_URL);
 await page.waitForFunction(()=>document.querySelector('output')?.textContent?.includes('runtime: running'));
  await page.evaluate(async()=>{
   localStorage.removeItem('sro:v1150:game-options:1');
   const entry=Array.from(document.scripts).find(s=>s.src&&new URL(s.src).pathname==='/src/bootstrap.ts');
   const {runtime}=await import(entry.src);runtime.dispose();
   const {createUi}=await import('/src/engine/runtime/ui/ui.ts'),{createRenderer}=await import('/src/engine/runtime/renderer/renderer.ts'),{createPlatform}=await import('/src/engine/runtime/platform/platform.ts');
   const jobs=new Map(),requests=[],commands=[],renderer=createRenderer(document.querySelector('canvas'));let serial=0,scene=null;
   const assets={available:()=>Math.max(0,32-jobs.size),request(url,limit,kind){const id=++serial;jobs.set(id,null);requests.push(url);fetch(url).then(async r=>{if(!r.ok)throw Error(r.status+' '+url);const result=kind==='png'?{kind:'image',image:await createImageBitmap(await r.blob())}:{kind:'bytes',buffer:await r.arrayBuffer()};if(jobs.has(id))jobs.set(id,result);else if(result.kind==='image')result.image.close();}).catch(e=>{if(jobs.has(id))jobs.set(id,{kind:'error',error:String(e)});});return id;},take(id){const result=jobs.get(id);if(result)jobs.delete(id);return result;},cancel(id){const result=jobs.get(id);if(result?.kind==='image')result.image.close();jobs.delete(id);}};
   let platform;const ui=createUi(assets,c=>commands.push(c),s=>{scene=s;renderer.setUi(s);},(id,image)=>renderer.setUiTexture(id,image),location.origin,'http://fixture.invalid',()=>{},()=>{},()=>1,()=>0,value=>platform.saveGameOptions(value));
   platform=createPlatform(document.querySelector('canvas'),document.querySelector('output'),()=>{},()=>{},()=>{},ui.event,ui.blocks);
   const state={session:{phase:'world',revision:1,character:'Fixture',characters:[{id:1,name:'Fixture',level:1,maxHp:200,maxMp:200}]},gameplay:{localGid:1,inventory:[],inventorySlotCount:45,equipmentSlotCount:13,vitals:[{gid:1,hp:200,mp:200}],casts:[]},entities:[],width:1200,height:900,worldReady:true};
   window.flagFixture={ui,renderer,platform,state,requests,commands,get scene(){return scene;},get pending(){return jobs.size;},draw(){const semantic=ui.step({...state},performance.now());if(semantic)platform.presentUi(semantic);renderer.frame({width:state.width,height:state.height});}};
   state.gameplay.eligibility={gm:false,pcRoomEvent:true};flagFixture.draw();
  });
  await page.waitForFunction(()=>{flagFixture.draw();return flagFixture.pending===0;},null,{timeout:20000});
  await page.evaluate(()=>{(document.activeElement??document.body).dispatchEvent(new KeyboardEvent('keydown',{code:'Backquote',key:'Backquote',shiftKey:true,bubbles:true}));});await page.evaluate(()=>flagFixture.draw());
  assert.equal(await page.locator('[data-ui-id="gm-input"]').count(),0);
  await page.evaluate(()=>{flagFixture.state.gameplay={...flagFixture.state.gameplay,eligibility:{gm:true,pcRoomEvent:false},revision:2};flagFixture.draw();});
  await page.evaluate(()=>window.dispatchEvent(new KeyboardEvent('keydown',{code:'Backquote',key:'Backquote'})));await page.evaluate(()=>flagFixture.draw());
  assert.equal(await page.locator('[data-ui-id="gm-input"]').count(),0);
  await page.evaluate(()=>{(document.activeElement??document.body).dispatchEvent(new KeyboardEvent('keydown',{code:'Backquote',key:'Backquote',shiftKey:true,bubbles:true}));});
  await page.waitForFunction(()=>{flagFixture.draw();const e=document.querySelector('[data-ui-id="gm-input"]');return e&&e.getBoundingClientRect().y>=96;});
  await page.locator('[data-ui-id="gm-input"]').fill('/FINDUSER Remo');await page.evaluate(()=>flagFixture.draw());
  await page.keyboard.press('Enter');await page.evaluate(()=>flagFixture.draw());
  assert.deepEqual(await page.evaluate(()=>flagFixture.commands.at(-1)),{kind:'gameplay',command:{kind:'gm-command',line:'/FINDUSER Remo'}});
  await page.evaluate(()=>{flagFixture.state.gameplay={...flagFixture.state.gameplay,revision:3,gmReplies:[{sequence:1,console:true,text:'-> Remo'},{sequence:2,console:false,text:'Remo (region 0x694F)'}]};flagFixture.draw();});
  const input=page.locator('[data-ui-id="gm-input"]');
  await input.fill('/INVISIBLE');await page.evaluate(()=>flagFixture.draw());
  await input.press('Enter');await page.evaluate(()=>flagFixture.draw());
  const commandCount=await page.evaluate(()=>flagFixture.commands.length);
  for(const [key,value] of [['ArrowUp','/INVISIBLE'],['ArrowUp','/FINDUSER Remo'],['ArrowUp','/FINDUSER Remo'],['ArrowDown','/FINDUSER Remo'],['ArrowDown','/INVISIBLE'],['ArrowDown','/INVISIBLE']]){
   await input.press(key);await page.evaluate(()=>flagFixture.draw());assert.equal(await input.inputValue(),value);
  }
  assert.equal(await page.evaluate(()=>flagFixture.commands.length),commandCount,'history navigation never executes commands or recalls server output');
  // The inline catalogue: the list draws, arrows move the selection, Tab
  // inserts the codename and Enter runs it with the row's default amount.
  await page.evaluate(()=>{
   flagFixture.state.gameplay={...flagFixture.state.gameplay,gmCatalog:{
    items:[
     {codename:'ITEM_CH_BLADE_01_A',name:'Copper Blade',typeFlags:42},
     {codename:'ITEM_ETC_HP_POTION_05',name:'HP recovery potion (X-large)',typeFlags:6380}
    ],
    monsters:[
     {codename:'MOB_CH_MANGNYANG',name:'Mangyang',monsterType:0},
     {codename:'MOB_CH_TIGERWOMAN',name:'Tiger Woman',monsterType:3}
    ]
   }};
   flagFixture.draw();
  });
  const gmInput=page.locator('[data-ui-id="gm-input"]');
  await gmInput.fill('/MAKEITEM ');await page.evaluate(()=>flagFixture.draw());
  assert.ok(await page.locator('[data-ui-id^="gm-helper-row:"]').count()>0,'catalogue rows draw above the prompt');
  await gmInput.press('ArrowDown');await page.evaluate(()=>flagFixture.draw());
  await gmInput.press('Tab');await page.evaluate(()=>flagFixture.draw());
  assert.equal(await gmInput.inputValue(),'/MAKEITEM ITEM_ETC_HP_POTION_05 ');
  await gmInput.press('Enter');await page.evaluate(()=>flagFixture.draw());
  assert.deepEqual(await page.evaluate(()=>flagFixture.commands.at(-1)),{kind:'gameplay',command:{kind:'gm-command',line:'/MAKEITEM ITEM_ETC_HP_POTION_05 1'}});
  // F3 opens Items; Right moves to Monsters and then Uniques, whose row is a
  // /LOADMONSTER command.
  await gmInput.press('F3');await page.evaluate(()=>flagFixture.draw());
  await gmInput.press('ArrowRight');await page.evaluate(()=>flagFixture.draw());
  await gmInput.press('ArrowRight');await page.evaluate(()=>flagFixture.draw());
  const uniqueRows=await page.locator('[data-ui-id^="gm-helper-row:"]').evaluateAll(nodes=>nodes.map(node=>node.textContent));
  assert.deepEqual(uniqueRows,['MOB_CH_TIGERWOMAN Tiger Woman']);
  await gmInput.press('Tab');await page.evaluate(()=>flagFixture.draw());
  assert.equal(await gmInput.inputValue(),'/LOADMONSTER MOB_CH_TIGERWOMAN ');
  await gmInput.press('Enter');await page.evaluate(()=>flagFixture.draw());
  assert.deepEqual(await page.evaluate(()=>flagFixture.commands.at(-1)),{kind:'gameplay',command:{kind:'gm-command',line:'/LOADMONSTER MOB_CH_TIGERWOMAN 1'}});
  assert.deepEqual(await page.evaluate(()=>flagFixture.ui.stats().failed),[]);assert.equal(await page.evaluate(()=>flagFixture.renderer.error()),null);
  await mkdir('temp/artifacts/intro-event-gm',{recursive:true});await page.screenshot({path:'temp/artifacts/intro-event-gm/console.png'});
  await page.evaluate(()=>{(document.activeElement??document.body).dispatchEvent(new KeyboardEvent('keydown',{code:'Backquote',key:'Backquote',shiftKey:true,bubbles:true}));});
  await page.waitForFunction(()=>{flagFixture.draw();return !document.querySelector('[data-ui-id="gm-input"]');});
  await page.evaluate(()=>{(document.activeElement??document.body).dispatchEvent(new KeyboardEvent('keydown',{code:'Backquote',key:'Backquote',shiftKey:true,bubbles:true}));});await page.waitForFunction(()=>{flagFixture.draw();return !!document.querySelector('[data-ui-id="gm-input"]');});
  await page.evaluate(()=>{flagFixture.state.gameplay={...flagFixture.state.gameplay,eligibility:{gm:false,pcRoomEvent:true},revision:4};flagFixture.draw();});
  assert.equal(await page.locator('[data-ui-id="gm-input"]').count(),0);
  await page.evaluate(()=>{flagFixture.ui.dispose();flagFixture.platform.dispose();flagFixture.renderer.dispose();});
 }finally{await browser.close();}
});
