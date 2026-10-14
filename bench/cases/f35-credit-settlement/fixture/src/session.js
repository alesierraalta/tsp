import { Catalog } from './catalog.js';
import { Basket } from './basket.js';
import { Ledger } from './ledger.js';
import { invoice } from './invoice.js';
import { summarize } from './report.js';

export class Session {
  constructor() {
    this.catalog = new Catalog();
    this.basket = new Basket();
    this.ledger = new Ledger();
  }

  register(code, microcredits, description) {
    this.catalog.define(code, microcredits, description);
    return this;
  }

  add(code, units, note) {
    this.catalog.lookup(code);
    return this.basket.add(code, units, note);
  }

  preview(reference) {
    return invoice(this.catalog, this.basket, reference);
  }

  checkout(reference) {
    const value = this.preview(reference);
    this.ledger.post(value);
    this.basket.clear();
    return value;
  }

  summary() {
    return summarize(this.ledger);
  }

  remove(lineId) {
    return this.basket.remove(lineId);
  }

  balance() {
    return this.ledger.balance();
  }

  products() {
    return this.catalog.entries();
  }
}
