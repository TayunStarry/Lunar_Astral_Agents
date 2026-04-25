const fs = require('fs');
const path = require('path');

const targetFile = path.join(__dirname, 'hierarchy', 'assets', 'agentSystem.js');

let content = fs.readFileSync(targetFile, 'utf-8');
const lines = content.split('\n');
const filteredLines = lines.filter(line => !/^\s*export\s*\{/.test(line));

if (lines.length !== filteredLines.length) {
    fs.writeFileSync(targetFile, filteredLines.join('\n'), 'utf-8');
} 
else console.log('No export line found');
