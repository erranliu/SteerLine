package backend

func NewQuestionRoute(implementationKey string, codecVersion int, identity QuestionRouteIdentity, payload []byte) (ImplementationDataEnvelope, error) {
	return NewImplementationDataEnvelope(ImplementationDataEnvelope{
		CodecVersion: codecVersion, Purpose: QuestionRouteAnswerPurpose, ImplementationKey: implementationKey,
		ProjectID: identity.ProjectID, WorkflowRunID: identity.WorkflowRunID, ExecutionID: identity.ExecutionID,
		AttemptID: identity.AttemptID, Generation: identity.Generation, TurnID: identity.TurnID,
		QuestionID: identity.QuestionID,
	}, payload)
}

func (e ImplementationDataEnvelope) ValidateIdentity(identity QuestionRouteIdentity) error {
	return e.ValidateQuestionIdentity(identity)
}
