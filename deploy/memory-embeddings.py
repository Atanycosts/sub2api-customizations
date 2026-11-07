#!/usr/bin/env python3
"""One CPU encoder shared by independent product-user memory libraries.

The loopback API implements OpenAI embeddings. It has no account credentials,
memory store, retrieval cache, or persistence of request text. Inputs longer
than 512 tokens are encoded in complete windows and combined as a normalized
weighted mean; the distinct model ID makes this policy part of index identity.
"""

import argparse
import hashlib
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import threading

import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer


class Encoder:
    def __init__(self, directory, model, dimensions):
        for name, digest in {
            'tokenizer.json': '48cea5d44424912a6fd1ea647bf4fe50b55ab8b1e5879c3275f80e339e8fae26',
            'onnx/model_quantized.onnx': '15b717c382bcb518ba457b93ea6850ede7f4f1cd8937454aa06972366cd19bcc',
        }.items():
            if hashlib.sha256((directory / name).read_bytes()).hexdigest() != digest:
                raise RuntimeError('Pinned encoder resource checksum mismatch: ' + name)
        self.model = model
        self.dimensions = dimensions
        self.tokenizer = Tokenizer.from_file(str(directory / 'tokenizer.json'))
        self.tokenizer.no_truncation()
        self.tokenizer.no_padding()
        special = self.tokenizer.encode('').ids
        self.prefix, self.suffix = special[0], special[-1]
        options = ort.SessionOptions()
        options.intra_op_num_threads = 1
        options.inter_op_num_threads = 1
        self.session = ort.InferenceSession(str(directory / 'onnx/model_quantized.onnx'),
                                            options, providers=['CPUExecutionProvider'])
        self.input_names = {item.name for item in self.session.get_inputs()}
        self.slot = threading.Lock()
        probe, _ = self.encode(['shared memory encoder startup probe'])
        if len(probe[0]) != dimensions:
            raise RuntimeError('Unexpected encoder dimensions')

    def encode(self, texts):
        encoded = self.tokenizer.encode_batch(texts, add_special_tokens=False)
        total = sum(len(item.ids) + 2 for item in encoded)
        if any(len(item.ids) + 2 > 8192 for item in encoded) or total > 16384:
            raise ValueError('input token capacity exceeded')
        vectors = []
        for item in encoded:
            # BGE uses CLS pooling; include the original tokenizer's special
            # tokens in every window. Every content token contributes.
            combined = np.zeros(self.dimensions, dtype=np.float64)
            for offset in range(0, max(1, len(item.ids)), 510):
                ids = item.ids[offset:offset + 510]
                tokens = np.asarray([[self.prefix, *ids, self.suffix]], dtype=np.int64)
                feed = {'input_ids': tokens, 'attention_mask': np.ones_like(tokens)}
                if 'token_type_ids' in self.input_names:
                    feed['token_type_ids'] = np.zeros_like(tokens)
                output = self.session.run(None, feed)[0]
                vector = np.asarray(output[0, 0] if output.ndim == 3 else output[0], dtype=np.float64)
                norm = np.linalg.norm(vector)
                if not np.isfinite(norm) or norm == 0 or vector.shape != (self.dimensions,):
                    raise RuntimeError('Encoder returned an invalid vector')
                combined += (vector / norm) * max(1, len(ids))
            norm = np.linalg.norm(combined)
            if not np.isfinite(norm) or norm == 0:
                raise RuntimeError('Encoder returned an invalid aggregate')
            vectors.append((combined / norm).astype(float).tolist())
        return vectors, total


class Handler(BaseHTTPRequestHandler):
    encoder = None

    def log_message(self, *_):
        pass  # Text, credentials and provider response bodies never enter logs.

    def send_json(self, status, body):
        data = json.dumps(body, separators=(',', ':')).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path != '/health':
            self.send_json(404, {'error': 'not_found'})
            return
        self.send_json(200, {'status': 'ok', 'model': self.encoder.model,
                             'dimensions': self.encoder.dimensions, 'max_input_tokens': 8192})

    def do_POST(self):
        self.connection.settimeout(15)
        if self.path != '/v1/embeddings':
            self.send_json(404, {'error': 'not_found'})
            return
        try:
            length = int(self.headers.get('Content-Length', '0'))
            if length <= 0 or length > 1024 * 1024:
                self.send_json(413, {'error': 'body_capacity_exceeded'})
                return
            data = json.loads(self.rfile.read(length))
            if not isinstance(data, dict):
                self.send_json(400, {'error': 'invalid_input'})
                return
            if data.get('model') != self.encoder.model:
                self.send_json(400, {'error': 'model_mismatch'})
                return
            texts = data.get('input')
            if isinstance(texts, str):
                texts = [texts]
            if not isinstance(texts, list) or not 1 <= len(texts) <= 16 or any(not isinstance(t, str) for t in texts):
                self.send_json(400, {'error': 'invalid_input'})
                return
            if not self.encoder.slot.acquire(timeout=2):
                self.send_json(503, {'error': 'encoder_busy'})
                return
            try:
                vectors, tokens = self.encoder.encode(texts)
            finally:
                self.encoder.slot.release()
        except (ValueError, KeyError, TypeError):
            self.send_json(400, {'error': 'invalid_input_or_token_capacity'})
            return
        except Exception:
            self.send_json(503, {'error': 'encoder_unavailable'})
            return
        self.send_json(200, {'object': 'list', 'model': self.encoder.model,
                             'data': [{'object': 'embedding', 'index': i, 'embedding': vector}
                                      for i, vector in enumerate(vectors)],
                             'usage': {'prompt_tokens': tokens, 'total_tokens': tokens}})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path, required=True)
    parser.add_argument('--model', required=True)
    parser.add_argument('--dimensions', type=int, required=True)
    parser.add_argument('--port', type=int, default=15168)
    args = parser.parse_args()
    Handler.encoder = Encoder(args.directory, args.model, args.dimensions)
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    server.daemon_threads = True
    server.serve_forever()


if __name__ == '__main__':
    main()
