import type { Decision } from "../../shared/api/types";

export const replyMaxBytes = 2000;

export function structuredAnswer(
  request: Decision["request"],
  note: string,
): Record<string, string[]> | undefined {
  if (!request.has_native_questions || !note.trim().startsWith("{"))
    return undefined;

  const answers = JSON.parse(note);
  if (
    !answers ||
    Array.isArray(answers) ||
    Object.values(answers).some(
      (value) =>
        !Array.isArray(value) || value.some((item) => typeof item !== "string"),
    )
  )
    throw new Error("多题答案须为问题 ID 对应字符串数组的 JSON");
  return answers;
}

export function replyBytes(
  request: Decision["request"],
  option: string,
  note: string,
) {
  let text = note;
  let answers: Record<string, string[]> | undefined;
  try {
    answers = structuredAnswer(request, note);
  } catch {
    // Count the draft text while the user is still editing invalid JSON.
    return new TextEncoder().encode(note).length;
  }
  if (answers) {
    const questions = request.native_questions ?? [];
    const values = Object.entries(answers).map(([id, parts]) => {
      const options = questions.find((question) => question.id === id)?.options;
      return [
        id,
        parts.map(
          (part) =>
            options?.find(
              (choice) => choice.ID === part || choice.Label === part,
            )?.Label ?? part,
        ),
      ] as const;
    });
    if (questions.length === 1 && values.length === 1) {
      text = values[0][1].join("\n");
    } else {
      // Match Go's JSON escaping for the multi-question text sent to Runner.
      text = JSON.stringify(Object.fromEntries(values)).replace(
        /[<>&\u2028\u2029]/g,
        (character) =>
          "\\u" + character.charCodeAt(0).toString(16).padStart(4, "0"),
      );
    }
  } else if (option) {
    const label = request.options?.find(
      (choice) => choice.ID === option,
    )?.Label;
    if (label) text = /\P{White_Space}/u.test(note) ? label + "\n" + note : label;
  }
  return new TextEncoder().encode(text).length;
}
