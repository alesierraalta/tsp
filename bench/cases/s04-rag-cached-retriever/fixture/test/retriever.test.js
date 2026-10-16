import test from "node:test";
import assert from "node:assert/strict";
import { Retriever } from "../src/retriever.js";

// A toy model: one dimension per letter of "abc".
const letters = (text) => ["a", "b", "c"].map((l) => [...text].filter((c) => c === l).length);

test("search ranks the principal's notes by similarity", () => {
  const r = new Retriever(letters);
  r.add("alice", "n1", "aaa");
  r.add("alice", "n2", "bbb");
  assert.deepEqual(r.search("alice", "aab", 1), ["n1"]);
});

test("search never returns another owner's note", () => {
  const r = new Retriever(letters);
  r.add("alice", "n1", "aaa");
  r.add("bob", "n2", "aaa");
  assert.deepEqual(r.search("bob", "aaa"), ["n2"]);
});

test("a note added after a search is found by the next search", () => {
  const r = new Retriever(letters);
  r.add("alice", "n1", "aaa");
  r.search("alice", "bbb");
  r.add("alice", "n2", "bbb");
  assert.deepEqual(r.search("alice", "bbb", 1), ["n2"]);
});

test("reindex keeps every note searchable", () => {
  const r = new Retriever(letters);
  r.add("alice", "n1", "aaa");
  r.reindex((t) => [t.length, 1]);
  assert.deepEqual(r.search("alice", "anything"), ["n1"]);
});
