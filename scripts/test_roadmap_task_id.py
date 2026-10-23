"""Offline regression for independent draft/formal document validation."""

import os
import tempfile
import unittest
from pathlib import Path

import roadmap_task_id as ids


class DocumentBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.directory = self.root / 'SteerlineFlow' / 'Roadmap'
        self.directory.mkdir(parents=True)
        (self.root / 'SteerlineFlow' / 'spec').mkdir()
        (self.root / 'SteerlineFlow' / 'plans').mkdir()
        self.records = {7: 'task-seven', 8: 'task-eight'}
        self.write('execution-map.md', '')

    def write(self, name, content):
        path = self.directory / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding='utf-8')

    def contract_row(self, number=7, stem='seven', base='drafts'):
        name = self.records[number]
        for suffix, directory in (('spec', 'spec'), ('plan', 'plans')):
            path = self.root / 'SteerlineFlow' / directory / f'{stem}.{suffix}.md'
            path.write_text(f'任务 ID：{number}；任务名称：{name}。\n', encoding='utf-8')
        spec = os.path.relpath(self.root / 'SteerlineFlow' / 'spec' / f'{stem}.spec.md', self.directory / base).replace('\\', '/')
        plan = os.path.relpath(self.root / 'SteerlineFlow' / 'plans' / f'{stem}.plan.md', self.directory / base).replace('\\', '/')
        return (f'| {number} | {name} | scope | none | '
                f'[Spec]({spec}) · [Plan]({plan}) |\n')

    def test_empty_formal_does_not_admit_draft(self):
        self.write('drafts/roadmap.md', self.contract_row())
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 0))
        self.assertEqual(ids.document_tasks(self.directory / 'execution-map.md'), {})

    def test_partial_draft_needs_no_execution_contract(self):
        self.write('drafts/future-topic.md', '| 7 | task-seven | discussing | none | not written |\n')
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 0))

    def test_formal_task_can_exist_without_draft_duplicate(self):
        self.write('execution-map.md', self.contract_row(base=''))
        self.assertEqual(ids.check_documents(self.root, self.records), (0, 1))

    def test_draft_and_formal_can_use_different_document_snapshots(self):
        self.write('drafts/roadmap.md', self.contract_row(stem='draft'))
        self.write('execution-map.md', self.contract_row(stem='formal', base=''))
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 1))

    def test_formal_task_cannot_borrow_contract_from_draft(self):
        self.write('drafts/roadmap.md', self.contract_row())
        self.write('execution-map.md', '| 7 | task-seven | scope | none | missing |\n')
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)

    def test_formal_identity_mismatch_is_rejected(self):
        self.write('execution-map.md', self.contract_row(base='').replace('task-seven', 'other-name'))
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)

    def test_formal_duplicate_is_rejected(self):
        row = self.contract_row(base='')
        self.write('execution-map.md', row + row)
        with self.assertRaises(ValueError):
            ids.check_documents(self.root, self.records)

    def test_multiple_arbitrary_draft_files_are_checked(self):
        self.write('drafts/runtime-direction.md', self.contract_row())
        self.write('drafts/planning-direction.md', self.contract_row(8, 'eight'))
        self.assertEqual(ids.check_documents(self.root, self.records), (2, 0))

    def test_nested_draft_links_resolve_from_their_own_directory(self):
        self.write('drafts/topic/proposal.md', self.contract_row(base='drafts/topic'))
        self.assertEqual(ids.check_documents(self.root, self.records), (1, 0))

    def test_alternative_drafts_do_not_become_formal_members(self):
        self.write('drafts/option-a.md', self.contract_row(stem='option-a'))
        self.write('drafts/option-b.md', self.contract_row(stem='option-b'))
        self.assertEqual(ids.check_documents(self.root, self.records), (2, 0))


if __name__ == '__main__':
    unittest.main()
