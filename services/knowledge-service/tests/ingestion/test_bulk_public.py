import hashlib
import json
import os

import psycopg
import pytest

from app.ingestion.bulk_public import bulk_import_batch
from app.ingestion.sqlite_public import rollback_batch


@pytest.mark.skipif(not os.getenv('TEST_DATABASE_URL'), reason='TEST_DATABASE_URL is not configured')
def test_bulk_import_preserves_identity_batches_and_atomicity(tmp_path):
    url = os.environ['TEST_DATABASE_URL']
    source = 'bulk-pg-fixture:book:1'
    batch = 'bulk-pg-first'
    def row(index, text):
        return {'id': f'{source}:chunk:{index}:part:0:v1', 'public_source_id': source,
                'dataset_id': 'bulk-pg-fixture', 'title': 'Test', 'content': text,
                'content_hash': hashlib.sha256(text.encode()).hexdigest(), 'index_version': 'v1'}
    def write(name, rows):
        path = tmp_path / name
        path.write_text(''.join(json.dumps(record) + '\n' for record in rows))
        return path
    records = [row(i, f'Test content {i}') for i in range(250)]
    file = write('first.jsonl', records)
    try:
        with psycopg.connect(url) as db:
            db.execute("INSERT INTO public_knowledge_sources(id,dataset_id,source_kind,source_record_id,title,enabled) VALUES(%s,'bulk-pg-fixture','book',1,'Test',true)", (source,))
        assert bulk_import_batch(url, file, batch) == {'inserted': 250, 'skipped': 0}
        assert bulk_import_batch(url, file, 'bulk-pg-retry') == {'inserted': 0, 'skipped': 250}
        with psycopg.connect(url) as db:
            assert db.execute('SELECT DISTINCT import_batch_id FROM knowledge_documents WHERE public_source_id=%s', (source,)).fetchall() == [(batch,)]
            assert db.execute('SELECT imported_chunks FROM public_knowledge_sources WHERE id=%s', (source,)).fetchone() == (250,)
            assert db.execute("SELECT count(*) FROM knowledge_documents WHERE public_source_id=%s AND public_search_vector=to_tsvector('simple',search_text)", (source,)).fetchone() == (250,)
        changed = write('changed.jsonl', [row(999, 'New'), row(0, 'Changed')])
        with pytest.raises(ValueError, match='changed content'):
            bulk_import_batch(url, changed, 'bulk-pg-changed')
        conflict = write('conflict.jsonl', [row(888, 'One'), row(888, 'Two')])
        with pytest.raises(ValueError, match='changed content'):
            bulk_import_batch(url, conflict, 'bulk-pg-conflict')
        duplicate = write('duplicate.jsonl', [row(666, 'Duplicate'), row(777, 'Duplicate')])
        assert bulk_import_batch(url, duplicate, 'bulk-pg-duplicate') == {'inserted': 1, 'skipped': 1}
        with pytest.raises(ValueError, match='immutable'):
            bulk_import_batch(url, duplicate, batch)
        bad = row(555, 'Wrong hash')
        bad['content_hash'] = 'bad'
        with pytest.raises(ValueError, match='hash'):
            bulk_import_batch(url, write('bad.jsonl', [bad]), 'bulk-pg-bad')
        with psycopg.connect(url) as db:
            assert db.execute("SELECT count(*) FROM public_knowledge_import_batches WHERE batch_id IN ('bulk-pg-changed','bulk-pg-conflict','bulk-pg-bad')").fetchone() == (0,)
        assert rollback_batch(url, 'bulk-pg-retry') == {'deleted': 0}
        assert rollback_batch(url, batch) == {'deleted': 250}
        with pytest.raises(ValueError, match='rolled back'):
            bulk_import_batch(url, file, batch)
    finally:
        with psycopg.connect(url) as db:
            db.execute('DELETE FROM knowledge_documents WHERE public_source_id=%s', (source,))
            db.execute('DELETE FROM public_knowledge_sources WHERE id=%s', (source,))
            db.execute("DELETE FROM public_knowledge_import_batches WHERE batch_id LIKE 'bulk-pg-%%'")
