在受管工作区从 context.pr.head_revision 续作原 PR，合入当前目标分支，保留双方有效改动并验证。发现原 PR 的远端 head 已变化就停止。只允许快进推送原维护分支，保留原 PR 和正文。回传原 PR URL、实际完整 head/base 和验证结果。
