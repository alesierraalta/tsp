function foldDottedI(value) {
  return value.toLowerCase();
}

function foldSharpS(value) {
  return value.toLowerCase();
}

export function createLookup(entries) {
  const folded = new Map();
  for (const [key, value] of Object.entries(entries)) {
    folded.set(canonicalize(foldSharpS(foldDottedI(key))), value);
  }
  return {
    get(key) {
      return folded.get(canonicalize(foldSharpS(foldDottedI(key))));
    },
  };
}

function canonicalize(value) {
  return value.normalize('NFC');
}
