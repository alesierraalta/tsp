import test from "node:test";
import assert from "node:assert/strict";
import { Index } from "../src/index.js";

test("search finds the principal's own note", () => {
  const i = new Index();
  i.add("alice", "a1", "Budget for March");
  assert.deepEqual(i.search("alice", "budget"), ["a1"]);
});

test("search ranks notes sharing more query terms first", () => {
  const i = new Index();
  i.add("alice", "a1", "march budget");
  i.add("alice", "a2", "march budget review");
  assert.deepEqual(i.search("alice", "budget review"), ["a2", "a1"]);
});

test("search never returns another owner's note", () => {
  const i = new Index();
  i.add("alice", "a1", "salary review");
  i.add("bob", "b1", "salary review");
  assert.deepEqual(i.search("bob", "salary"), ["b1"]);
});

test("delete refuses a note that belongs to someone else", () => {
  const i = new Index();
  i.add("alice", "a1", "salary");
  assert.equal(i.delete("bob", "a1"), false);
  assert.deepEqual(i.search("alice", "salary"), ["a1"]);
});
