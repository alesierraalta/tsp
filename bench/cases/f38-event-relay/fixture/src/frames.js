export function encode(frame) {
  if (frame.type === 'keep-alive') {
    return `: keep-alive ${frame.cursor}\n\n`;
  }
  if (frame.type !== 'event') throw new TypeError('unknown frame type');
  return [
    `id: ${frame.cursor}`,
    `event: ${frame.topic}`,
    `data: ${JSON.stringify(frame.payload)}`,
    '',
    '',
  ].join('\n');
}

export async function pipeFrames(stream, sink) {
  let written = 0;
  try {
    for await (const frame of stream) {
      await sink.write(encode(frame));
      written++;
    }
    return written;
  } finally {
    await sink.end();
  }
}

export function headers() {
  return {
    'content-type': 'text/event-stream',
    'cache-control': 'no-cache',
    connection: 'keep-alive',
  };
}
