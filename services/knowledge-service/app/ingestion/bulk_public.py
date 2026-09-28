"""Transactional COPY staging for large, immutable public-knowledge batches."""
import hashlib
from pathlib import Path

import psycopg
from psycopg.types.json import Jsonb

from app.ingestion.sqlite_public import (
    _NAMESPACE, _dataset_id, _file_digest, _jsonl, _refresh_counts,
)
from app.retrieval.tokenization import search_tokens


def bulk_import_batch(database_url: str, input_path: str | Path, batch_id: str) -> dict[str, int]:
    if not _NAMESPACE.fullmatch(batch_id):
        raise ValueError('invalid batch-id')
    digest = _file_digest(input_path)
    source_datasets = {}
    for row in _jsonl(input_path):
        dataset = _dataset_id(row['dataset_id'])
        source = row['public_source_id']
        if not source.startswith(dataset + ':'):
            raise ValueError('document dataset does not match source namespace')
        source_datasets[source] = dataset
    total = 0
    with psycopg.connect(database_url) as db, db.cursor() as cursor:
        cursor.execute('SELECT pg_advisory_xact_lock(hashtext(%s))', ('public-import:' + batch_id,))
        cursor.execute('SELECT file_hash,status FROM public_knowledge_import_batches WHERE batch_id=%s FOR UPDATE', (batch_id,))
        existing = cursor.fetchone()
        if existing:
            if existing[1] == 'rolled_back':
                raise ValueError('batch-id was rolled back; use a new batch-id')
            if existing[0] != digest:
                raise ValueError('batch-id already belongs to a different immutable import file')
        else:
            cursor.execute("INSERT INTO public_knowledge_import_batches(batch_id,file_hash,status) VALUES(%s,%s,'completed')", (batch_id, digest))
        if source_datasets:
            cursor.execute('SELECT id,dataset_id FROM public_knowledge_sources WHERE id=ANY(%s::text[]) ORDER BY id FOR NO KEY UPDATE', (sorted(source_datasets),))
            if dict(cursor.fetchall()) != source_datasets:
                raise ValueError('sources must be registered in the declared dataset before import')
        cursor.execute('''CREATE TEMP TABLE public_import_stage (
            id TEXT, title TEXT, content TEXT, source TEXT, locator JSONB, metadata JSONB,
            content_hash TEXT, index_version TEXT, public_source_id TEXT, search_text TEXT
        ) ON COMMIT DROP''')
        with cursor.copy('COPY public_import_stage FROM STDIN') as copy:
            for row in _jsonl(input_path):
                source = row['public_source_id']
                if source_datasets.get(source) != _dataset_id(row['dataset_id']):
                    raise ValueError('document dataset does not match source namespace')
                if not row['id'].startswith(source + ':chunk:') or not row['id'].endswith(':' + row['index_version']):
                    raise ValueError('document id must include source/chunk and cleaning version')
                content_hash = hashlib.sha256(row['content'].encode()).hexdigest()
                if row['content_hash'] != content_hash:
                    raise ValueError('document content hash mismatch')
                metadata = {**row.get('metadata', {}), 'import_file_hash': digest,
                            'content_is_untrusted_reference': True}
                copy.write_row((row['id'], row['title'], row['content'], row.get('source', row['title']),
                                Jsonb(row.get('locator', {})), Jsonb(metadata), content_hash,
                                row['index_version'], source, search_tokens(row['title'] + '\n' + row['content'])))
                total += 1
        cursor.execute('''SELECT id FROM public_import_stage GROUP BY id
            HAVING count(DISTINCT (content_hash, public_source_id, index_version)) > 1 LIMIT 1''')
        if cursor.fetchone():
            raise ValueError('same document id has changed content; use a new cleaning version')
        cursor.execute('''SELECT s.id FROM public_import_stage s JOIN knowledge_documents d ON d.id=s.id
            WHERE ROW(d.content_hash,d.public_source_id,d.index_version)
                IS DISTINCT FROM ROW(s.content_hash,s.public_source_id,s.index_version) LIMIT 1''')
        if cursor.fetchone():
            raise ValueError('same document id has changed content; use a new cleaning version')
        cursor.execute('''INSERT INTO knowledge_documents
            (id,library_kind,release_id,enneagram_type,safety_level,title,content,source,locator,metadata,
             content_hash,embedding_model,index_version,embedding,public_source_id,import_batch_id,search_text,public_search_vector)
            SELECT id,'public',NULL,NULL,0,title,content,source,locator,metadata,
                content_hash,'',index_version,NULL,public_source_id,%s,search_text,to_tsvector('simple',search_text)
            FROM public_import_stage ORDER BY public_source_id,id
            ON CONFLICT DO NOTHING RETURNING public_source_id''', (batch_id,))
        inserted_sources = cursor.fetchall()
        inserted = len(inserted_sources)
        cursor.execute('''SELECT s.id FROM public_import_stage s JOIN knowledge_documents d ON d.id=s.id
            WHERE ROW(d.content_hash,d.public_source_id,d.index_version)
                IS DISTINCT FROM ROW(s.content_hash,s.public_source_id,s.index_version) LIMIT 1''')
        if cursor.fetchone():
            raise ValueError('concurrent same-id import has changed content')
        if _file_digest(input_path) != digest:
            raise ValueError('import input file changed while importing')
        _refresh_counts(cursor, {row[0] for row in inserted_sources})
    return {'inserted': inserted, 'skipped': total - inserted}
