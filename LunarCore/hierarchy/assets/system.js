const timers = new Map();
let nextTimerId = 1;
function setTimeout(callback, delay, ...args) {
    const timerId = nextTimerId++;
    let cancelled = false;
    timers.set(timerId, () => {
        cancelled = true;
    });
    (async () => {
        await _waiter(delay);
        if (!cancelled) {
            callback(...args);
        }
        timers.delete(timerId);
    })();
    return timerId;
}
async function Log(message) {
    return await _log(message);
}

Log('awakenAgent');
setTimeout(() => {
    Log('awakenAgent-000');
}, 1000);
