import {test} from 'node:test';
import assert from 'node:assert/strict';
import {Timers} from './timers.js';
test('delivers absolute and relative deadlines once and in registration order', () => {
 const c={wall:1000,mono:0},t=new Timers(c),out=[];
 t.at(1010,()=>out.push('a')); t.after(10,()=>out.push('b'));
 c.wall+=9; c.mono+=9; assert.equal(t.poll(),0);
 c.wall++; c.mono++; assert.equal(t.poll(),2);
 assert.deepEqual(out,['a','b']); assert.equal(t.poll(),0);
});
test('callbacks can schedule another deadline', () => {
 const c={wall:200,mono:50},t=new Timers(c); let count=0;
 t.after(0,()=>{count++;t.after(1,()=>count++);});
 assert.equal(t.poll(),1); assert.equal(count,1);
 c.wall++;c.mono++; assert.equal(t.poll(),1);assert.equal(count,2);
});
