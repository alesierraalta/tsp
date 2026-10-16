import test from "node:test";
import assert from "node:assert/strict";
import {build,consume} from "../src/builder.js";
test("builds consumable frames without mutating metadata",()=>{
 const rows=[{sequence:1,name:"a",value:10},{sequence:2,name:"b",value:20},{sequence:3,name:"a",value:30}];
 const before=structuredClone(rows);
 assert.deepEqual(build(rows),[[{name:"a",value:10},{name:"b",value:20},{name:"a",value:30}]]);
 assert.deepEqual(consume(build(rows)),{a:30,b:20});
 assert.deepEqual(rows,before);
});
test("empty and single files",()=>{
 assert.deepEqual(build([]),[[]]);assert.deepEqual(consume(build([])),{});
 assert.deepEqual(consume(build([{sequence:7,name:"enabled",value:false}])),{enabled:false});
});
test("descending metadata",()=>{
 const rows=[{sequence:2,name:"b",value:2},{sequence:1,name:"a",value:1}];
 assert.deepEqual(consume(build(rows,"desc")),{b:2,a:1});
});
