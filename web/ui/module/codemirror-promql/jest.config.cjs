/** @type {import('ts-jest/dist/types').InitialOptionsTsJest} */
module.exports = {
    preset: 'ts-jest',
    extensionsToTreatAsEsm: ['.ts'],
    testEnvironment: 'node',
    setupFiles: [
        './setupJest.cjs'
    ],
    globals: {
        'ts-jest': {
            useESM: true,
        },
    },
    moduleNameMapper: {
        'lezer-promql': '<rootDir>/../../node_modules/@prometheus-io/lezer-promql/dist/index.cjs',
        // @codemirror/state's CJS build require()s @marijn/find-cluster-break, whose "main"
        // is ESM-only. Jest ignores the package's "require" export condition and loads the ESM
        // entry, so pin it to the CommonJS build explicitly.
        '^@marijn/find-cluster-break$': '<rootDir>/../../node_modules/@marijn/find-cluster-break/dist/index.cjs'
    },
    transformIgnorePatterns: ["<rootDir>/../../node_modules/(?!@prometheus-io/lezer-promql)/"]
};
