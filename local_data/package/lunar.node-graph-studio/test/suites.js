/**
 * 测试清单与两个入口共用的汇总逻辑。
 *   · Node：test/run.js
 *   · 浏览器：test/index.html（在琉璃里打开包页面后可直接看到结果）
 */

import { routerSuite } from "./suites/router.test.js";
import { portsSuite } from "./suites/ports.test.js";
import { graphSuite, documentSuite, historySuite, snapSuite, layoutSuite, textSuite, commandsSuite } from "./suites/model.test.js";

export const ALL_SUITES = [
	routerSuite,
	portsSuite,
	graphSuite,
	documentSuite,
	historySuite,
	snapSuite,
	layoutSuite,
	textSuite,
	commandsSuite,
];
