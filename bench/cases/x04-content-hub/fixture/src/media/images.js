const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
const GIF_SIGNATURES = [Buffer.from('GIF87a'), Buffer.from('GIF89a')];

export function isPng(buffer) {
  return buffer.length >= 8 && buffer.subarray(0, 8).equals(PNG_SIGNATURE);
}

export function isGif(buffer) {
  return buffer.length >= 6 && GIF_SIGNATURES.some((signature) => buffer.subarray(0, 6).equals(signature));
}

export function isJpeg(buffer) {
  return buffer.length >= 3 && buffer[0] === 0xff && buffer[1] === 0xd8 && buffer[2] === 0xff;
}

export function pngSize(buffer) {
  if (!isPng(buffer) || buffer.length < 24) return undefined;
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) };
}

export function gifSize(buffer) {
  if (!isGif(buffer) || buffer.length < 10) return undefined;
  return { width: buffer.readUInt16LE(6), height: buffer.readUInt16LE(8) };
}

export function jpegSize(buffer) {
  if (!isJpeg(buffer)) return undefined;
  let index = 2;
  while (index + 9 < buffer.length) {
    if (buffer[index] !== 0xff) {
      index += 1;
      continue;
    }
    const marker = buffer[index + 1];
    const length = buffer.readUInt16BE(index + 2);
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
      return { height: buffer.readUInt16BE(index + 5), width: buffer.readUInt16BE(index + 7) };
    }
    index += 2 + length;
  }
  return undefined;
}

export function svgSize(text) {
  const source = String(text ?? '');
  const width = /width="(\d+)"/.exec(source);
  const height = /height="(\d+)"/.exec(source);
  const viewBox = /viewBox="0 0 (\d+) (\d+)"/.exec(source);
  if (width && height) return { width: Number(width[1]), height: Number(height[1]) };
  if (viewBox) return { width: Number(viewBox[1]), height: Number(viewBox[2]) };
  return undefined;
}

export function imageSize(name, buffer) {
  const ext = String(name ?? '').toLowerCase().split('.').pop();
  switch (ext) {
    case 'png':
      return pngSize(buffer);
    case 'gif':
      return gifSize(buffer);
    case 'jpg':
    case 'jpeg':
      return jpegSize(buffer);
    case 'svg':
      return svgSize(buffer.toString('utf8'));
    default:
      return undefined;
  }
}

export function aspectRatio(size) {
  if (!size || !size.width || !size.height) return 1;
  return size.width / size.height;
}

export function scaleTo(size, limit) {
  if (!size) return undefined;
  const scale = Math.min(1, limit.width / size.width, limit.height / size.height);
  if (scale >= 1) return { width: size.width, height: size.height, scaled: false };
  return {
    width: Math.max(1, Math.round(size.width * scale)),
    height: Math.max(1, Math.round(size.height * scale)),
    scaled: true,
  };
}

export function fitsWithin(size, limit) {
  if (!size) return false;
  return size.width <= limit.width && size.height <= limit.height;
}

export function describeSize(size) {
  if (!size) return 'unknown';
  return `${size.width}x${size.height}`;
}

export const PRESETS = {
  thumb: { width: 160, height: 160 },
  card: { width: 480, height: 480 },
  hero: { width: 1600, height: 900 },
};

export function crop(size, box) {
  if (!size) return undefined;
  const ratio = size.width / size.height;
  const target = box.width / box.height;
  if (ratio > target) {
    const width = Math.round(size.height * target);
    return { width, height: size.height, x: Math.round((size.width - width) / 2), y: 0 };
  }
  const height = Math.round(size.width / target);
  return { width: size.width, height, x: 0, y: Math.round((size.height - height) / 2) };
}

export function megapixels(size) {
  if (!size) return 0;
  return Math.round(((size.width * size.height) / 1_000_000) * 100) / 100;
}

export function isLarge(size, limit = 1_000_000) {
  if (!size) return false;
  return size.width * size.height > limit;
}

export function nearestPreset(size) {
  if (!size) return 'thumb';
  const pixels = size.width * size.height;
  if (pixels > 1_000_000) return 'hero';
  if (pixels > 120_000) return 'card';
  return 'thumb';
}

export function describeSizes(entries) {
  return entries.map((entry) => ({
    name: entry.name,
    size: entry.dimensions ? describeSize(entry.dimensions) : 'unknown',
    megapixels: megapixels(entry.dimensions),
    preset: nearestPreset(entry.dimensions),
  }));
}
