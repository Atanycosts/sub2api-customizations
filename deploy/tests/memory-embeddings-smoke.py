#!/usr/bin/env python3
"""Check the real loopback encoder without reading or writing a user library."""

import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import math
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', default='http://127.0.0.1:15168')
    args = parser.parse_args()
    model = 'sub2api-bge-small-zh-windowed-v1'

    def call(payload):
        request = urllib.request.Request(
            args.base_url + '/v1/embeddings', data=json.dumps(payload).encode(),
            headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=25) as response:
                return response.status, json.load(response)
        except urllib.error.HTTPError as error:
            return error.code, json.load(error)

    status, data = call({'model': model, 'input': [
        '用户喜欢简洁的中文回复。', '使用简短中文回答是用户的偏好。',
        'Redis数据库连接失败。', '长文本完整编码测试。' * 300,
    ]})
    assert status == 200 and len(data['data']) == 4
    vectors = [row['embedding'] for row in data['data']]
    for vector in vectors:
        assert len(vector) == 512 and all(math.isfinite(value) for value in vector)
        assert abs(math.sqrt(sum(value * value for value in vector)) - 1) < 1e-6
    related = sum(a * b for a, b in zip(vectors[0], vectors[1]))
    unrelated = sum(a * b for a, b in zip(vectors[0], vectors[2]))
    assert related > unrelated
    status, changed = call({'model': model, 'input': '长文本完整编码测试。' * 70 + '数据库服务器维护。' * 230})
    assert status == 200
    assert changed['data'][0]['embedding'] != vectors[3], 'tail tokens must affect the embedding'
    for payload in [None, [], {'model': 'unknown', 'input': 'test'},
                    {'model': model, 'input': []}, {'model': model, 'input': ['x'] * 17},
                    {'model': model, 'input': '超出容量' * 4000}]:
        assert call(payload)[0] == 400
    with ThreadPoolExecutor(max_workers=3) as pool:
        results = list(pool.map(lambda _: call({'model': model, 'input': '同时测试共享推理'}), range(3)))
    assert all(status == 200 for status, _ in results)
    print(json.dumps({'numeric_capacity_concurrency_checks': 'passed',
                      'related_cosine': round(related, 4), 'unrelated_cosine': round(unrelated, 4)}))


if __name__ == '__main__':
    main()
