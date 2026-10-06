import { execFileSync } from 'node:child_process';
import { expect, it } from 'vitest';

const existingValidity = '2026-10-10T00:15:42.123+02:00';
const timezoneCases = [
  {
    timeZone: 'Europe/Berlin', existingDate: '2026-10-10',
    samples: [
      ['2026-10-10', '2026-10-09T22:00:00.000Z', -120],
      ['2026-03-29', '2026-03-28T23:00:00.000Z', -60],
      ['2026-03-30', '2026-03-29T22:00:00.000Z', -120],
      ['2026-10-25', '2026-10-24T22:00:00.000Z', -120],
      ['2026-10-26', '2026-10-25T23:00:00.000Z', -60],
    ],
  },
  {
    timeZone: 'America/Los_Angeles', existingDate: '2026-10-09',
    samples: [
      ['2026-10-10', '2026-10-10T07:00:00.000Z', 420],
      ['2026-03-08', '2026-03-08T08:00:00.000Z', 480],
      ['2026-03-09', '2026-03-09T07:00:00.000Z', 420],
      ['2026-11-01', '2026-11-01T07:00:00.000Z', 420],
      ['2026-11-02', '2026-11-02T08:00:00.000Z', 480],
    ],
  },
] as const;

it.each(timezoneCases)('round-trips local calendar dates and preserves unchanged validity in $timeZone across DST', ({ timeZone, existingDate, samples }) => {
  // Vitest uses worker threads; start Node with TZ rather than mutating a worker's environment.
  // Bundle the real source in memory; do not depend on version-specific Node TypeScript flags.
  const result = JSON.parse(execFileSync(process.execPath, ['--input-type=module', '--eval', `
    import { buildSync } from 'esbuild';
    import { resolve } from 'node:path';
    const bundle = buildSync({
      entryPoints: [resolve(process.cwd(), 'src/components/consent/consentDraft.ts')],
      bundle: true, platform: 'node', format: 'esm', write: false,
    });
    // The bundle is generated in memory at runtime and has no static module path.
    const { createConsentDraft } = await import('data:text/javascript;base64,' + Buffer.from(bundle.outputFiles[0].contents).toString('base64'));
    const now = new Date(2026, 0, 1, 12);
    Date.now = () => now.getTime();
    const permissionSets = [{
      requirement_type: 'optional',
      permission_set: {
        id: 'extra', name: 'Extra access', description: 'Optional mail',
        service_scopes: [{ service_id: 'mail', requirement_type: 'optional' }],
      },
    }];
    const load = (validUntil) => createConsentDraft({
      permissionSets, context: 'console',
      existingGrant: validUntil ? { valid_until: validUntil, granted_permission_sets: {} } : null,
    });
    const roundTrips = ${JSON.stringify(samples.map(([date]) => date))}.map((date) => {
      const validUntil = load().setDuration('custom', date).toGrantRequest(now).valid_until;
      const loaded = load(validUntil);
      const permissionEdit = loaded.setPermissionSet('extra', true);
      return {
        date: loaded.customDate, validUntil, offset: new Date(validUntil).getTimezoneOffset(),
        loadedDirty: loaded.dirty, permissionEditDirty: permissionEdit.dirty,
        unchangedValidity: loaded.toGrantRequest(now).valid_until,
        permissionEditValidity: permissionEdit.toGrantRequest(now).valid_until,
        serviceEditValidity: permissionEdit.setService('extra', 'mail', false).toGrantRequest(now).valid_until,
        dateReselectValidity: loaded.setDuration('custom', loaded.customDate).toGrantRequest(now).valid_until,
      };
    });
    const existing = load(${JSON.stringify(existingValidity)});
    console.log(JSON.stringify({
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      roundTrips,
      existing: { date: existing.customDate, request: existing.setPermissionSet('extra', true).toGrantRequest(now) },
    }));
  `], { encoding: 'utf8', env: { ...process.env, TZ: timeZone } }));

  expect(result.timeZone).toBe(timeZone);
  expect(result.roundTrips).toEqual(samples.map(([date, validUntil, offset]) => ({
    date, validUntil, offset, loadedDirty: false, permissionEditDirty: true,
    unchangedValidity: validUntil,
    permissionEditValidity: validUntil,
    serviceEditValidity: validUntil,
    dateReselectValidity: validUntil,
  })));
  expect(result.existing).toEqual({
    date: existingDate,
    request: { granted_permission_sets: { extra: ['mail'] }, valid_until: existingValidity },
  });
});
