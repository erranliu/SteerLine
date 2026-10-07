"""Offline regression for independent draft/formal document validation."""

import tempfile
import unittest
from pathlib import Path

import roadmap_task_id as ids


class DocumentBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.directory = self.root / 'docs' / 'roadmap'
        self.directory.mkdir(parents=True)
        (self.root / 'docs' / 'spec').mkdir()
        self.records = {7: 'task-seven', 8: 'task-eight'}
        self.write('roadmap.md', '')
        self.write('execution-map.md', '')

    def write(self, name, content):
        (self.directory / name).write_text(content, encoding='utf-8')

    def contract_row(self, number=7, stem='seven'):
        name = self.records[number]
        for suffix in ('spec', 'plan'):
            path = self.root / 'docs' / 'spec' / f'{stem}.{suffix}.md'
            path.write_text(f'任务 ID：{number}；任务名称：{name}。\n', encoding='utf-8')
        return (f'| {number} | {name} | scope | none | '
                f'[Spec](../spec/{stem}.spec.md) · [Plan](../spec/{stem}.plan.md) |\n')

    def test_empty_formal_does_not_admit_draft(self):
        self.write('roadmap.md', self.contract_row())
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 0))
        self.assertEqual(ids.document_tasks(self.directory / 'execution-map.md'), {})

    def test_partial_draft_needs_no_execution_contract(self):
        self.write('roadmap.md', '| 7 | task-seven | discussing | none | not written |\n')
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 0))

    def test_formal_task_can_exist_without_draft_duplicate(self):
        self.write('execution-map.md', self.contract_row())
        self.assertEqual(ids.check_documents(self.root, self.records), (0, 1))

    def test_draft_and_formal_can_use_different_document_snapshots(self):
        self.write('roadmap.md', self.contract_row(stem='draft'))
        self.write('execution-map.md', self.contract_row(stem='formal'))
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 1))

    def test_formal_task_cannot_borrow_contract_from_draft(self):
        self.write('roadmap.md', self.contract_row())
        self.write('execution-map.md', '| 7 | task-seven | scope | none | missing |\n')
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)

    def test_formal_identity_mismatch_is_rejected(self):
        self.write('execution-map.md', self.contract_row().replace('task-seven', 'other-name'))
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)

    def test_formal_duplicate_is_rejected(self):
        row = self.contract_row()
        self.write('execution-map.md', row + row)
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)


if __name__ == '__main__':
    unittest.main()
