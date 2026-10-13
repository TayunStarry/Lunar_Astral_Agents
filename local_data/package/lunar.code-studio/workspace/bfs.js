/**
 * 广度优先搜索 (BFS) 实现
 * @param {Object} graph - 邻接表形式的图，例如：{'A': ['B', 'C'], 'B': ['A', 'D'], ...}
 * @param {string} startNode - 起始节点
 * @returns {string[]} - 遍历顺序
 */
function bfs(graph, startNode) {
  if (!graph[startNode]) {
    return ["错误：起始节点不存在"];
  }

  const visited = new Set(); // 记录已访问的节点
  const queue = [startNode]; // 队列，用于存储待访问的节点
  const result = []; // 存储遍历顺序

  visited.add(startNode);

  while (queue.length > 0) {
    // 从队列头部取出第一个节点（FIFO）
    const currentNode = queue.shift();
    result.push(currentNode);

    // 获取当前节点的所有邻居
    const neighbors = graph[currentNode] || [];
    for (const neighbor of neighbors) {
      if (!visited.has(neighbor)) {
        visited.add(neighbor); // 标记为已访问
        queue.push(neighbor);   // 加入队列
      }
    }
  }

  return result;
}

// --- 测试用例 ---
// 构造一个简单的图：
// A --- B --- D
// |     |
// C --- E
const myGraph = {
  'A': ['B', 'C'],
  'B': ['A', 'D', 'E'],
  'C': ['A', 'E'],
  'D': ['B'],
  'E': ['B', 'C']
};

console.log("从节点 A 开始的 BFS 遍历结果:", bfs(myGraph, 'A'));
console.log("从节点 B 开始的 BFS 遍历结果:", bfs(myGraph, 'B'));
