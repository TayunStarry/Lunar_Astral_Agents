// ==== molang-runtime.js — MoLang 表达式解释器 ====

/**
 * Bedrock MoLang 表达式语言的最小化解释器
 * 参考：https://bedrock.dev/docs/molang
 *
 * 支持特性：
 *   - 数字字面量（含负数、小数）
 *   - 变量访问：q.* / query.* / v.* / variable.* / t.* / temp.* / c.* / context.*
 *   - 数学函数：math.cos / math.sin / math.tan / math.abs / math.floor / math.ceil /
 *              math.sqrt / math.pow / math.min / math.max / math.clamp / math.pi /
 *              math.round / math.log / math.exp / math.atan / math.atan2 / math.acos / math.asin
 *   - 算术：+ - * / %
 *   - 比较：> < >= <= == !=
 *   - 逻辑：&& || !
 *   - 三元：cond ? a : b
 *   - 括号、分号（多语句，返回最后一个）
 *   - 布尔字面量：true / false
 *   - null（求值为 0）
 *
 * 性能：表达式 AST 编译后缓存，重复求值仅遍历树
 */

// ==== Token 类型 ====
const TOK = {
    NUMBER: 'NUMBER',
    IDENT: 'IDENT',
    STRING: 'STRING',
    LPAREN: 'LPAREN',
    RPAREN: 'RPAREN',
    LBRACKET: 'LBRACKET',
    RBRACKET: 'RBRACKET',
    COMMA: 'COMMA',
    DOT: 'DOT',
    QUESTION: 'QUESTION',
    COLON: 'COLON',
    SEMICOLON: 'SEMICOLON',
    PLUS: 'PLUS',
    MINUS: 'MINUS',
    STAR: 'STAR',
    SLASH: 'SLASH',
    PERCENT: 'PERCENT',
    GT: 'GT',
    LT: 'LT',
    GE: 'GE',
    LE: 'LE',
    EQ: 'EQ',
    NE: 'NE',
    ASSIGN: 'ASSIGN',
    AND: 'AND',
    OR: 'OR',
    NOT: 'NOT',
    EOF: 'EOF'
};

// ==== Tokenizer ====
function tokenize(src) {
    const tokens = [];
    let i = 0;
    const n = src.length;
    while (i < n) {
        const c = src[i];
        // 空白字符
        if (c === ' ' || c === '\t' || c === '\n' || c === '\r') { i++; continue; }
        // 字符串字面量（MoLang 罕见，但 query 函数参数可能使用）
        if (c === "'") {
            i++;
            let str = '';
            while (i < n && src[i] !== "'") {
                if (src[i] === '\\' && i + 1 < n) { str += src[i + 1]; i += 2; continue; }
                str += src[i++];
            }
            i++; // 跳过结束引号
            tokens.push({ type: TOK.STRING, value: str });
            continue;
        }
        // 数字（含小数）
        if ((c >= '0' && c <= '9') || (c === '.' && i + 1 < n && src[i + 1] >= '0' && src[i + 1] <= '9')) {
            let num = '';
            while (i < n && ((src[i] >= '0' && src[i] <= '9') || src[i] === '.')) {
                num += src[i++];
            }
            tokens.push({ type: TOK.NUMBER, value: parseFloat(num) });
            continue;
        }
        // 标识符（字母或下划线开头）
        if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c === '_') {
            let id = '';
            while (i < n && ((src[i] >= 'a' && src[i] <= 'z') || (src[i] >= 'A' && src[i] <= 'Z') || (src[i] >= '0' && src[i] <= '9') || src[i] === '_')) {
                id += src[i++];
            }
            tokens.push({ type: TOK.IDENT, value: id });
            continue;
        }
        // 双字符操作符
        const two = src.substr(i, 2);
        if (two === '>=') { tokens.push({ type: TOK.GE }); i += 2; continue; }
        if (two === '<=') { tokens.push({ type: TOK.LE }); i += 2; continue; }
        if (two === '==') { tokens.push({ type: TOK.EQ }); i += 2; continue; }
        if (two === '!=') { tokens.push({ type: TOK.NE }); i += 2; continue; }
        if (two === '&&') { tokens.push({ type: TOK.AND }); i += 2; continue; }
        if (two === '||') { tokens.push({ type: TOK.OR }); i += 2; continue; }
        // 单字符操作符
        switch (c) {
            case '(': tokens.push({ type: TOK.LPAREN }); break;
            case ')': tokens.push({ type: TOK.RPAREN }); break;
            case '[': tokens.push({ type: TOK.LBRACKET }); break;
            case ']': tokens.push({ type: TOK.RBRACKET }); break;
            case ',': tokens.push({ type: TOK.COMMA }); break;
            case '.': tokens.push({ type: TOK.DOT }); break;
            case '?': tokens.push({ type: TOK.QUESTION }); break;
            case ':': tokens.push({ type: TOK.COLON }); break;
            case ';': tokens.push({ type: TOK.SEMICOLON }); break;
            case '+': tokens.push({ type: TOK.PLUS }); break;
            case '-': tokens.push({ type: TOK.MINUS }); break;
            case '*': tokens.push({ type: TOK.STAR }); break;
            case '/': tokens.push({ type: TOK.SLASH }); break;
            case '%': tokens.push({ type: TOK.PERCENT }); break;
            case '>': tokens.push({ type: TOK.GT }); break;
            case '<': tokens.push({ type: TOK.LT }); break;
            case '!': tokens.push({ type: TOK.NOT }); break;
            case '=': tokens.push({ type: TOK.ASSIGN }); break;
            default:
                // 未知字符，跳过（容错）
                break;
        }
        i++;
    }
    tokens.push({ type: TOK.EOF });
    return tokens;
}

// ==== AST 节点类型 ====
// { type: 'num', value }
// { type: 'var', path: ['q', 'life_time'] }
// { type: 'call', path: ['math', 'cos'], args: [ast] }
// { type: 'unary', op, operand }
// { type: 'binary', op, left, right }
// { type: 'ternary', cond, then, else }
// { type: 'bool', value }

// ==== Parser（递归下降） ====
class Parser {
    constructor(tokens) {
        this.tokens = tokens;
        this.pos = 0;
    }

    peek() { return this.tokens[this.pos]; }
    next() { return this.tokens[this.pos++]; }
    expect(type) {
        const t = this.tokens[this.pos];
        if (t.type !== type) throw new Error(`MoLang 解析错误：期望 ${type}，得到 ${t.type}`);
        this.pos++;
        return t;
    }
    match(type) {
        if (this.tokens[this.pos].type === type) { this.pos++; return true; }
        return false;
    }

    parse() {
        // 顶层：多语句（分号分隔），返回最后一个
        const stmts = [this.parseStatement()];
        while (this.peek().type === TOK.SEMICOLON) {
            this.next();
            if (this.peek().type === TOK.EOF) break;
            stmts.push(this.parseStatement());
        }
        this.expect(TOK.EOF);
        if (stmts.length === 1) return stmts[0];
        return { type: 'block', stmts };
    }

    /**
     * 解析语句：可能是赋值或表达式
     */
    parseStatement() {
        const expr = this.parseExpression();
        // 赋值：var = expr
        if (this.peek().type === TOK.ASSIGN && expr.type === 'var') {
            this.next(); // 消费 '='
            const value = this.parseExpression();
            return { type: 'assign', target: expr.path, value };
        }
        return expr;
    }

    parseExpression() {
        return this.parseTernary();
    }

    parseTernary() {
        const cond = this.parseOr();
        if (this.match(TOK.QUESTION)) {
            const then = this.parseExpression();
            this.expect(TOK.COLON);
            const els = this.parseExpression();
            return { type: 'ternary', cond, then, els };
        }
        return cond;
    }

    parseOr() {
        let left = this.parseAnd();
        while (this.peek().type === TOK.OR) {
            this.next();
            const right = this.parseAnd();
            left = { type: 'binary', op: '||', left, right };
        }
        return left;
    }

    parseAnd() {
        let left = this.parseEquality();
        while (this.peek().type === TOK.AND) {
            this.next();
            const right = this.parseEquality();
            left = { type: 'binary', op: '&&', left, right };
        }
        return left;
    }

    parseEquality() {
        let left = this.parseComparison();
        while (this.peek().type === TOK.EQ || this.peek().type === TOK.NE) {
            const op = this.next().type === TOK.EQ ? '==' : '!=';
            const right = this.parseComparison();
            left = { type: 'binary', op, left, right };
        }
        return left;
    }

    parseComparison() {
        let left = this.parseAddition();
        while ([TOK.GT, TOK.LT, TOK.GE, TOK.LE].includes(this.peek().type)) {
            const t = this.next().type;
            const op = { [TOK.GT]: '>', [TOK.LT]: '<', [TOK.GE]: '>=', [TOK.LE]: '<=' }[t];
            const right = this.parseAddition();
            left = { type: 'binary', op, left, right };
        }
        return left;
    }

    parseAddition() {
        let left = this.parseMultiplication();
        while (this.peek().type === TOK.PLUS || this.peek().type === TOK.MINUS) {
            const op = this.next().type === TOK.PLUS ? '+' : '-';
            const right = this.parseMultiplication();
            left = { type: 'binary', op, left, right };
        }
        return left;
    }

    parseMultiplication() {
        let left = this.parseUnary();
        while ([TOK.STAR, TOK.SLASH, TOK.PERCENT].includes(this.peek().type)) {
            const t = this.next().type;
            const op = { [TOK.STAR]: '*', [TOK.SLASH]: '/', [TOK.PERCENT]: '%' }[t];
            const right = this.parseUnary();
            left = { type: 'binary', op, left, right };
        }
        return left;
    }

    parseUnary() {
        if (this.peek().type === TOK.NOT) {
            this.next();
            return { type: 'unary', op: '!', operand: this.parseUnary() };
        }
        if (this.peek().type === TOK.MINUS) {
            this.next();
            return { type: 'unary', op: '-', operand: this.parseUnary() };
        }
        return this.parsePostfix();
    }

    parsePostfix() {
        return this.parsePrimary();
    }

    parsePrimary() {
        const t = this.peek();
        // 括号表达式
        if (t.type === TOK.LPAREN) {
            this.next();
            const expr = this.parseExpression();
            this.expect(TOK.RPAREN);
            return expr;
        }
        // 数字
        if (t.type === TOK.NUMBER) {
            this.next();
            return { type: 'num', value: t.value };
        }
        // 字符串字面量（用于 query 函数参数，求值时返回 0）
        if (t.type === TOK.STRING) {
            this.next();
            return { type: 'string', value: t.value };
        }
        // 标识符：可能是变量、函数调用、布尔字面量
        if (t.type === TOK.IDENT) {
            this.next();
            // 布尔字面量
            if (t.value === 'true') return { type: 'num', value: 1 };
            if (t.value === 'false') return { type: 'num', value: 0 };
            if (t.value === 'null') return { type: 'num', value: 0 };
            // 构建路径：IDENT (. IDENT)*
            const path = [t.value];
            while (this.peek().type === TOK.DOT) {
                this.next();
                const part = this.expect(TOK.IDENT);
                path.push(part.value);
            }
            // 函数调用？
            if (this.peek().type === TOK.LPAREN) {
                this.next();
                const args = [];
                if (this.peek().type !== TOK.RPAREN) {
                    args.push(this.parseExpression());
                    while (this.match(TOK.COMMA)) {
                        args.push(this.parseExpression());
                    }
                }
                this.expect(TOK.RPAREN);
                return { type: 'call', path, args };
            }
            // 变量访问
            return { type: 'var', path };
        }
        // 数组访问 [index] ?（MoLang 极少使用，降级为 0）
        if (t.type === TOK.LBRACKET) {
            this.next();
            this.parseExpression(); // 消费内容
            this.expect(TOK.RBRACKET);
            return { type: 'num', value: 0 };
        }
        throw new Error(`MoLang 解析错误：意外 token ${t.type}`);
    }
}

// ==== 数学函数表 ====
// 重要：Bedrock MoLang 的三角函数使用【度】而非弧度
// 参考 https://bedrock.dev/docs/molang：math.cos/sin/tan 入参为度，
// math.acos/asin/atan/atan2 返回值也为度
// 需在 JS Math.*（弧度）与 MoLang（度）之间做换算
const DEG2RAD = Math.PI / 180;
const RAD2DEG = 180 / Math.PI;
const MATH_FUNCS = {
    pi: () => Math.PI,
    cos: (x) => Math.cos(x * DEG2RAD),
    sin: (x) => Math.sin(x * DEG2RAD),
    tan: (x) => Math.tan(x * DEG2RAD),
    acos: (x) => Math.acos(x) * RAD2DEG,
    asin: (x) => Math.asin(x) * RAD2DEG,
    atan: (x) => Math.atan(x) * RAD2DEG,
    atan2: (y, x) => Math.atan2(y, x) * RAD2DEG,
    abs: (x) => Math.abs(x),
    floor: (x) => Math.floor(x),
    ceil: (x) => Math.ceil(x),
    round: (x) => Math.round(x),
    sqrt: (x) => Math.sqrt(x),
    pow: (x, y) => Math.pow(x, y),
    log: (x) => Math.log(x),
    exp: (x) => Math.exp(x),
    min: (a, b) => Math.min(a, b),
    max: (a, b) => Math.max(a, b),
    clamp: (x, lo, hi) => Math.max(lo, Math.min(hi, x)),
    lerp: (a, b, t) => a + (b - a) * t,
    random: () => Math.random(),
    mod: (a, b) => ((a % b) + b) % b
};

// ==== 命名空间别名 ====
const NS_ALIAS = {
    q: 'query',
    v: 'variable',
    t: 'temp',
    c: 'context'
};

// ==== 默认 query 变量 ====
const DEFAULT_QUERY = {
    life_time: 0,
    time: 0,
    state_time: 0,
    vertical_speed: 0,
    ground_speed: 0,
    target_x_rotation: 0,
    target_y_rotation: 0,
    body_y_rotation: 0,
    is_sneaking: 0,
    is_moving: 0,
    is_in_water: 0,
    is_sprinting: 0,
    is_on_ground: 1,
    all_animations_finished: 0,
    any_animation_finished: 0,
    is_first_person: 0,
    is_gliding: 0,
    is_swimming: 0,
    is_jumping: 0,
    is_falling: 0,
    is_sleeping: 0,
    is_riding: 0,
    item_is_charged: 0,
    health: 20,
    max_health: 20,
    y_head_rotation: 0,
    head_yaw: 0,
    body_yaw: 0,
    walk_distance: 0,
    move_speed: 0,
    lateral_speed: 0
};

// ==== MolangRuntime 主类 ====
export class MolangRuntime {
    constructor() {
        /** @type {Map<string, object>} 表达式 → AST 缓存 */
        this.cache = new Map();
        /** query.* 变量（q.* 别名） */
        this.query = { ...DEFAULT_QUERY };
        /** variable.* 变量（v.* 别名） */
        this.variables = {};
        /** temp.* 变量（t.* 别名） */
        this.temp = {};
        /** context.* 变量（c.* 别名） */
        this.context = {};

        // ==== 变更追踪（按键 M 模态框用） ====
        /** @type {Array<{key: string, value: any, timestamp: number}>} 最近变更记录 */
        this._changeLog = [];
        /** @type {number} 变更记录保留时长（毫秒） */
        this._changeLogTTL = 30000;
    }

    /**
     * 求值 MoLang 表达式
     * @param {string|number} expr
     * @returns {number}
     */
    eval(expr) {
        if (typeof expr === 'number') return expr;
        if (typeof expr !== 'string') return 0;
        const trimmed = expr.trim();
        if (trimmed === '') return 0;
        // 快速路径：纯数字
        if (/^-?\d+\.?\d*$/.test(trimmed)) {
            const n = parseFloat(trimmed);
            if (!isNaN(n)) return n;
        }
        // 编译并缓存
        let ast = this.cache.get(trimmed);
        if (!ast) {
            try {
                const tokens = tokenize(trimmed);
                ast = new Parser(tokens).parse();
            } catch (e) {
                // 解析失败：返回 0，避免崩溃
                console.warn('[MoLang] 解析失败:', trimmed, e.message);
                ast = { type: 'num', value: 0 };
            }
            this.cache.set(trimmed, ast);
        }
        try {
            return this._eval(ast);
        } catch (e) {
            console.warn('[MoLang] 求值失败:', trimmed, e.message);
            return 0;
        }
    }

    /**
     * 更新 query 上下文
     * @param {object} ctx
     */
    updateContext(ctx) {
        const now = Date.now();
        for (const key in ctx) {
            const newVal = ctx[key];
            const oldVal = this.query[key];
            // 仅在值实际变化时记录（NaN 视为无变化）
            if (oldVal !== newVal && !(Number.isNaN(oldVal) && Number.isNaN(newVal))) {
                this._changeLog.push({ key, value: newVal, timestamp: now });
            }
        }
        Object.assign(this.query, ctx);
    }

    /**
     * 获取最近 N 秒内变更的 molang 键值对（去重，保留最新值）
     * @param {number} seconds 时间窗口（秒），默认 30
     * @returns {Array<{key: string, value: any, age: number}>} 按名称长度+字母序排列
     */
    getRecentChanges(seconds = 30) {
        const cutoff = Date.now() - seconds * 1000;
        // 清理过期记录
        this._changeLog = this._changeLog.filter(e => e.timestamp >= cutoff);
        // 去重：每个 key 只保留最新记录
        const latest = new Map();
        for (const entry of this._changeLog) {
            latest.set(entry.key, entry);
        }
        // 转换为数组并按名称长度+字母序排列
        return Array.from(latest.values())
            .map(e => ({ key: e.key, value: e.value, age: (Date.now() - e.timestamp) / 1000 }))
            .sort((a, b) => a.key.length - b.key.length || a.key.localeCompare(b.key));
    }

    /**
     * 设置 variable
     */
    setVariable(name, value) {
        this.variables[name] = value;
    }

    /**
     * 重置临时变量和用户变量
     */
    reset() {
        this.temp = {};
        this.variables = {};
    }

    /**
     * 获取所有 MoLang 上下文变量的快照（用于调试面板实时显示）
     * @returns {{query: object, variables: object, temp: object, context: object}}
     */
    getAllContext() {
        return {
            query: { ...this.query },
            variables: { ...this.variables },
            temp: { ...this.temp },
            context: { ...this.context }
        };
    }

    /**
     * 重置所有（含 query）
     */
    resetAll() {
        this.query = { ...DEFAULT_QUERY };
        this.variables = {};
        this.temp = {};
        this.context = {};
    }

    /**
     * 求值 AST 节点
     * @param {object} node
     * @returns {number}
     * @private
     */
    _eval(node) {
        switch (node.type) {
            case 'num':
                return node.value;

            case 'string':
                // 字符串在数值上下文中返回 0（用于 query 函数参数占位）
                return 0;

            case 'bool':
                return node.value ? 1 : 0;

            case 'assign': {
                const value = this._eval(node.value);
                this._setVar(node.target, value);
                return value;
            }

            case 'block': {
                let v = 0;
                for (const s of node.stmts) v = this._eval(s);
                return v;
            }

            case 'var': {
                return this._resolveVar(node.path);
            }

            case 'call': {
                return this._callFunc(node.path, node.args);
            }

            case 'unary': {
                const v = this._eval(node.operand);
                if (node.op === '-') return -v;
                if (node.op === '!') return v ? 0 : 1;
                return v;
            }

            case 'binary': {
                // 短路求值
                if (node.op === '&&') {
                    return (this._eval(node.left) && this._eval(node.right)) ? 1 : 0;
                }
                if (node.op === '||') {
                    return (this._eval(node.left) || this._eval(node.right)) ? 1 : 0;
                }
                const a = this._eval(node.left);
                const b = this._eval(node.right);
                switch (node.op) {
                    case '+': return a + b;
                    case '-': return a - b;
                    case '*': return a * b;
                    case '/': return b === 0 ? 0 : a / b;
                    case '%': return b === 0 ? 0 : a % b;
                    case '>': return a > b ? 1 : 0;
                    case '<': return a < b ? 1 : 0;
                    case '>=': return a >= b ? 1 : 0;
                    case '<=': return a <= b ? 1 : 0;
                    case '==': return a === b ? 1 : 0;
                    case '!=': return a !== b ? 1 : 0;
                }
                return 0;
            }

            case 'ternary': {
                return this._eval(node.cond) ? this._eval(node.then) : this._eval(node.els);
            }

            default:
                return 0;
        }
    }

    /**
     * 解析变量路径
     * @param {string[]} path
     * @returns {number}
     * @private
     */
    _resolveVar(path) {
        if (path.length === 0) return 0;
        const ns = path[0];
        // 命名空间别名归一化
        const realNs = NS_ALIAS[ns] || ns;
        if (path.length < 2) {
            // 单段路径，直接当作变量
            return this.variables[ns] || 0;
        }
        const name = path[1];
        switch (realNs) {
            case 'query':
                return this.query[name] !== undefined ? this.query[name] : 0;
            case 'variable':
                return this.variables[name] !== undefined ? this.variables[name] : 0;
            case 'temp':
                return this.temp[name] !== undefined ? this.temp[name] : 0;
            case 'context':
                return this.context[name] !== undefined ? this.context[name] : 0;
            default:
                // 未知命名空间，返回 0
                return 0;
        }
    }

    /**
     * 设置变量值（用于赋值语句）
     * @param {string[]} path
     * @param {number} value
     * @private
     */
    _setVar(path, value) {
        if (path.length < 2) return;
        const ns = path[0];
        const realNs = NS_ALIAS[ns] || ns;
        const name = path[1];
        switch (realNs) {
            case 'variable':
                this.variables[name] = value;
                break;
            case 'temp':
                this.temp[name] = value;
                break;
            // query 和 context 为只读，不允许赋值
        }
    }

    /**
     * 调用函数
     * @param {string[]} path
     * @param {Array} argNodes
     * @returns {number}
     * @private
     */
    _callFunc(path, argNodes) {
        if (path.length < 2) return 0;
        const ns = path[0];
        const realNs = NS_ALIAS[ns] || ns;
        const name = path[1];
        // math.* 命名空间
        if (ns === 'math') {
            const fn = MATH_FUNCS[name];
            if (!fn) {
                console.warn(`[MoLang] 未知 math 函数: ${name}`);
                return 0;
            }
            const args = argNodes.map(a => this._eval(a));
            return fn(...args);
        }
        // query.* 函数调用（如 q.is_item_name_any）—— 无游戏状态，静默返回 0
        if (realNs === 'query') {
            return 0;
        }
        // 其他未知函数
        return 0;
    }

    /**
     * 清空 AST 缓存
     */
    clearCache() {
        this.cache.clear();
    }
}
