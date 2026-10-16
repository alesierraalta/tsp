export class Release {
  constructor(text) {
    this.text = text;
    this.revision = 1;
    this.reviewedRevision = 0;
    this.state = 'draft';
    this.suspended = false;
  }
  edit(text) {
    this.text = text;
    this.revision++;
  }
  approve() {
    this.reviewedRevision = this.revision;
    this.state = 'approved';
  }
  suspend() {
    this.suspended = true;
  }
  resume() {
    this.suspended = false;
  }
  publish() {
    if (this.reviewedRevision !== this.revision && this.state !== 'approved') return false;
    if (this.suspended) return false;
    this.state = 'published';
    return true;
  }
}
