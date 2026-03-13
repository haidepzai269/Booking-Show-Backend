import { pipeline } from '@xenova/transformers';
import http from 'http';

console.log("Loading model: Xenova/all-MiniLM-L6-v2...");
const extractor = await pipeline('feature-extraction', 'Xenova/all-MiniLM-L6-v2');

const server = http.createServer(async (req, res) => {
    if (req.method === 'POST' && req.url === '/embeddings') {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', async () => {
            try {
                const { inputs } = JSON.parse(body);
                console.log(`Generating embedding for: "${inputs.substring(0, 50)}..."`);
                const output = await extractor(inputs, { pooling: 'mean', normalize: true });
                
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify(Array.from(output.data)));
            } catch (err) {
                console.error(err);
                res.writeHead(500);
                res.end(JSON.stringify({ error: err.message }));
            }
        });
    } else {
        res.writeHead(404);
        res.end();
    }
});

const PORT = 5001;
server.listen(PORT, () => {
    console.log(`🚀 Local AI Embedding Service running on http://localhost:${PORT}`);
    console.log(`Use POST http://localhost:${PORT}/embeddings with {"inputs": "text"}`);
});
