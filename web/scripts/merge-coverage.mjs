import { access, readFile, readdir } from 'node:fs/promises'
import path from 'node:path'
import coverage from 'istanbul-lib-coverage'
import report from 'istanbul-lib-report'
import sourceMaps from 'istanbul-lib-source-maps'
import reports from 'istanbul-reports'

const webDir = path.resolve(import.meta.dirname, '..')
const repoRoot = path.resolve(webDir, '..')
const coverageDir = path.join(webDir, 'coverage')
const sourceDir = path.join(webDir, 'src')
const browserDir = path.resolve(repoRoot, process.env.E2E_WEB_COVERAGE_DIR || 'web/coverage/browser')

const { createCoverageMap } = coverage
const browserCoverage = createCoverageMap({})
const files = (await readdir(browserDir, { withFileTypes: true }))
  .filter(entry => entry.isFile() && entry.name.endsWith('.json'))
  .map(entry => entry.name)

if (files.length === 0) {
  throw new Error(`No browser coverage records found in ${browserDir}`)
}

for (const file of files) {
  const documents = JSON.parse(await readFile(path.join(browserDir, file), 'utf8'))
  if (!documents || Array.isArray(documents) || typeof documents !== 'object') {
    throw new Error(`Invalid browser coverage record: ${file}`)
  }
  for (const snapshot of Object.values(documents)) {
    browserCoverage.merge(createCoverageMap(snapshot))
  }
}

if (browserCoverage.files().length === 0) {
  throw new Error(`Browser coverage records in ${browserDir} contain no instrumented files`)
}
for (const file of browserCoverage.files()) {
  if (!browserCoverage.fileCoverageFor(file).data.inputSourceMap) {
    throw new Error(`Browser coverage lacks a source map for ${file}`)
  }
}

const sourceMapStore = sourceMaps.createSourceMapStore()
const remapped = await sourceMapStore.transformCoverage(browserCoverage)
sourceMapStore.dispose()
const mappedBrowser = createCoverageMap({})
for (const file of remapped.files()) {
  const absolute = path.resolve(webDir, file)
  const relative = path.relative(sourceDir, absolute)
  if (relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative) || !/\.tsx?$/.test(relative)) {
    continue
  }
  await access(absolute)
  const fileCoverage = remapped.fileCoverageFor(file).toJSON()
  mappedBrowser.addFileCoverage({ ...fileCoverage, path: absolute })
}

if (mappedBrowser.files().length === 0 || !mappedBrowser.files().some(file =>
  Object.values(mappedBrowser.fileCoverageFor(file).s).some(hits => hits > 0))) {
  throw new Error('Browser coverage did not map any executed web/src TypeScript')
}

const unitCoverage = createCoverageMap(JSON.parse(await readFile(path.join(coverageDir, 'coverage-final.json'), 'utf8')))
unitCoverage.merge(mappedBrowser)
reports.create('lcovonly', { projectRoot: repoRoot })
  .execute(report.createContext({ dir: coverageDir, coverageMap: unitCoverage }))
console.log(`Merged ${files.length} browser coverage records into web/coverage/lcov.info`)
