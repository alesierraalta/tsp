export class ToolRunner {
  constructor(tools, confirm) {
    this.tools = tools;
    this.confirm = confirm;
    this.approved = new Set();
  }

  async run(call) {
    const tool = this.tools[call.name];
    if (!tool) return { error: `unknown tool ${call.name}` };
    if (tool.sideEffect) {
      const key = JSON.stringify([call.name, call.args]);
      if (!this.approved.has(key)) {
        if (!(await this.confirm(call))) return { error: "declined" };
        this.approved.add(key);
      }
    }
    return { result: await tool.run(call.args) };
  }
}
