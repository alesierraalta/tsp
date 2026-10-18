import {test} from 'node:test';
import assert from 'node:assert/strict';
import {Settings} from './settings.js';
const setup = () => new Map([['a',{volume:4,flags:{},healthy:8}],['b',{volume:9,flags:{},healthy:6}]]);
test('updates all organizations and retains other settings', () => {
 const store=setup(), s=new Settings(store); store.get('a').flags.old=true;
 s.apply('enabled',true); s.apply('enabled',false);
 assert.deepEqual([...store.values()].map(x=>x.flags.enabled),[false,false]);
 assert.equal(store.get('a').flags.old,true);
 assert.deepEqual(s.busiest().map(x=>x.volume),[9,4]);
});
test('capacity changes respect the configured minimum', () => {
 const store=setup(),s=new Settings(store);
 assert.equal(s.retire('a',2,{minimumHealthy:5}),2);
 assert.equal(s.retire('a',8,{minimumHealthy:5}),1);
 assert.equal(s.retire('a',1,{minimumHealthy:5}),0);
});
