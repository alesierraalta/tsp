export class UploadSession {
  constructor() {
    this.state = 'ready';
    this.parts = new Set();
  }

  start() {
    if (this.state !== 'ready') return false;
    this.state = 'uploading';
    return true;
  }

  recordPart(id) {
    if (this.state !== 'uploading') return false;
    this.parts.add(id);
    return true;
  }

  pause() {
    if (this.state !== 'uploading') return false;
    this.state = 'paused';
    return true;
  }

  resume() {
    if (this.state === 'uploading') return false;
    this.state = 'uploading';
    return true;
  }

  fail() {
    if (this.state !== 'uploading') return false;
    this.state = 'failed';
    return true;
  }

  retry() {
    if (this.state !== 'failed') return false;
    this.parts = new Set(this.parts);
    this.state = 'uploading';
    return true;
  }

  complete(requiredParts) {
    if (this.state !== 'uploading' || this.parts.size < requiredParts) return false;
    this.state = 'completed';
    return true;
  }
}
