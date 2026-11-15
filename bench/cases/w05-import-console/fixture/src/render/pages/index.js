import { layout, renderNotice } from '../layout.js';
import { renderDashboard, renderDashboardEmpty, dashboardTitle } from './dashboard.js';
import { renderReportPage } from './report.js';
import { renderImportsPage, renderValidationPage, renderImportResult } from './imports.js';

export function renderPage(name, model = {}, config = {}) {
  const page = PAGES[name] ?? PAGES.notFound;
  const body = page.body(model, config);
  return layout({
    title: page.title(model),
    heading: page.heading?.(model) ?? page.title(model),
    body,
    user: model.user,
    messages: model.messages,
    active: page.active,
    mode: config.mode,
    pageData: model.pageData,
    footer: { note: config.title ?? 'Import console' },
  });
}

export const PAGES = {
  dashboard: {
    title: () => 'Dashboard',
    heading: (model) => dashboardTitle(model),
    active: '/',
    body: (model) => ((model.imports ?? []).length === 0 ? renderDashboardEmpty() : renderDashboard(model)),
  },
  imports: {
    title: () => 'Imports',
    active: '/imports',
    body: (model) => renderImportsPage(model),
  },
  validation: {
    title: () => 'Validation',
    active: '/imports',
    body: (model) => renderValidationPage(model),
  },
  result: {
    title: (model) => (model.ok ? 'Import complete' : 'Import failed'),
    active: '/imports',
    body: (model) => renderImportResult(model),
  },
  report: {
    title: (model) => `Report: ${model.dataset ?? 'rows'}`,
    active: '/reports',
    body: (model) => renderReportPage(model),
  },
  notFound: {
    title: () => 'Not found',
    active: '',
    body: (model) => renderNotice(model.message ?? 'That page does not exist.', { kind: 'error' }),
  },
};

export function pageNames() {
  return Object.keys(PAGES).sort();
}

export function renderNotFound(message) {
  return renderPage('notFound', { message });
}

export function renderErrorPage(error) {
  return renderPage('result', {
    ok: false,
    title: 'Something went wrong',
    message: error?.message ?? 'The page could not be rendered.',
  });
}
