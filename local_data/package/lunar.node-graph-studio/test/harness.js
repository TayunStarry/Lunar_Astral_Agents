/**
 * 极简测试框架（不依赖任何第三方库，Node 与浏览器共用）。
 *
 * 只做三件事：登记用例、断言、汇总结果。断言失败带上上下文，便于定位。
 */

export class AssertionError extends Error {
	constructor(message) {
		super(message);
		this.name = "AssertionError";
	}
}

export function createSuite(name) {
	const tests = [];
	return {
		name,
		tests,
		test(testName, fn) {
			tests.push({ name: testName, fn });
			return this;
		},
	};
}

export function assert(condition, message = "断言失败") {
	if (!condition) throw new AssertionError(message);
}

export function assertEqual(actual, expected, message = "") {
	if (actual !== expected) {
		throw new AssertionError(`${message || "值不相等"}：期望 ${fmt(expected)}，实际 ${fmt(actual)}`);
	}
}

export function assertDeepEqual(actual, expected, message = "") {
	const a = JSON.stringify(actual);
	const b = JSON.stringify(expected);
	if (a !== b) {
		throw new AssertionError(`${message || "结构不相等"}：期望 ${b}，实际 ${a}`);
	}
}

export function assertClose(actual, expected, epsilon = 1e-6, message = "") {
	if (!(Math.abs(actual - expected) <= epsilon)) {
		throw new AssertionError(`${message || "数值不接近"}：期望 ${fmt(expected)}±${epsilon}，实际 ${fmt(actual)}`);
	}
}

export function assertGreater(actual, threshold, message = "") {
	if (!(actual > threshold)) {
		throw new AssertionError(`${message || "应大于阈值"}：${fmt(actual)} 应 > ${fmt(threshold)}`);
	}
}

export function assertLess(actual, threshold, message = "") {
	if (!(actual < threshold)) {
		throw new AssertionError(`${message || "应小于阈值"}：${fmt(actual)} 应 < ${fmt(threshold)}`);
	}
}

export function assertThrows(fn, message = "应当抛出异常") {
	let threw = false;
	try {
		fn();
	} catch (err) {
		threw = true;
	}
	if (!threw) throw new AssertionError(message);
}

function fmt(value) {
	if (typeof value === "string") return JSON.stringify(value);
	if (typeof value === "object") {
		try {
			return JSON.stringify(value);
		} catch (err) {
			return String(value);
		}
	}
	return String(value);
}

/**
 * 运行全部套件。
 * @param {Array} suites
 * @param {{onResult?:Function}} [options]
 * @returns {{passed:number, failed:number, total:number, results:Array, failures:Array}}
 */
export function runSuites(suites, options = {}) {
	const results = [];
	let passed = 0;
	let failed = 0;

	for (const suite of suites) {
		for (const testCase of suite.tests) {
			const started = now();
			let error = null;
			try {
				testCase.fn();
			} catch (err) {
				error = err;
			}
			const duration = now() - started;
			const record = {
				suite: suite.name,
				name: testCase.name,
				ok: !error,
				error,
				duration,
			};
			results.push(record);
			if (error) failed += 1;
			else passed += 1;
			options.onResult?.(record);
		}
	}

	return {
		passed,
		failed,
		total: passed + failed,
		results,
		failures: results.filter((r) => !r.ok),
	};
}

function now() {
	if (typeof performance !== "undefined" && performance.now) return performance.now();
	return Date.now();
}

/** 构造一个用于测试的节点（带四向端口） */
export function makeNode(id, overrides = {}) {
	return {
		id,
		type: "process",
		x: 0,
		y: 0,
		w: 200,
		h: 100,
		title: id,
		text: "",
		color: "#5b8dff",
		shape: "round",
		fontSize: 13,
		ports: {
			in: { top: 1, right: 1, bottom: 1, left: 1 },
			out: { top: 1, right: 1, bottom: 1, left: 1 },
		},
		locked: false,
		collapsed: false,
		...overrides,
	};
}
