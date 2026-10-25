import axios from "axios";


const DEFAULT_ERROR_MESSAGE = "Не удалось выполнить запрос. Попробуйте ещё раз.";


const getErrorCode = ( error: unknown ): string | null => {
    if (!axios.isAxiosError(error)) { return null; }
    const data: unknown = error.response?.data;
    if (!data || typeof data !== "object") { return null; }
    const detail = "error" in data ? data.error : data;
    return detail && typeof detail === "object" && "code" in detail && typeof detail.code === "string" ? detail.code : null;
};


const explanations: Record<string, string> = {
    IMPORT_PREVIEW_STALE: "Данные счёта изменились после проверки. Загрузите файл заново и подтвердите новый предварительный просмотр. Данные не применены автоматически.",
    TRANSACTION_DUPLICATE_SUSPECTED: "Такая операция уже есть в импортированной выписке. Повторная ручная запись не создана.",
    FACTUAL_SOURCE_CONFLICT: "История счёта уже связана с другим источником. Импорт не изменил источник и операции.",
    SOURCE_BINDING_CONFLICT: "Для этого источника уже выбрана другая компания. Проверьте компанию в файле.",
    HISTORY_WINDOW_TOO_SHORT: "История короче 90 календарных дней. Загрузите более ранние операции; неизвестные дни не заполняются нулями.",
    INSUFFICIENT_HISTORY: "Истории недостаточно для расчёта. Проверьте период и ограничения загруженных данных.",
    HISTORY_RECONCILIATION_FAILED: "Операции не согласуются с остатками на конец дня. Проверьте пропуски и суммы выписки.",
    BALANCE_OBSERVATION_CONFLICT: "На эту дату уже есть остаток из выписки. Ручной якорь не может его перезаписать.",
    BALANCE_AS_OF_UNAVAILABLE: "На выбранную дату нет подтверждённого остатка на конец дня. Выберите другую дату или добавьте отдельный датированный остаток.",
    DEPENDENCY_UNAVAILABLE: "Финансовый сервис временно недоступен. Результат не получен; повторите проверку позже.",
    VERSION_CONFLICT: "Данные изменены другим действием. Обновите версии и проверьте введённые поля перед новой отправкой.",
    CSRF_INVALID: "Защитный токен отклонён. Обновите страницу перед повторной отправкой.",
};


const getErrorMessage = ( error: unknown ): string => {
    if (axios.isAxiosError(error)) {
        const explanation = explanations[getErrorCode(error) || ""];
        if (explanation) { return explanation; }
        const data: unknown = error.response?.data;

        if (data && typeof data === "object") {
            const detail = "error" in data ? data.error : data;

            if (detail && typeof detail === "object" && "message" in detail && typeof detail.message === "string" && detail.message.trim()) {
                return detail.message.trim();
            }
        }

        return DEFAULT_ERROR_MESSAGE;
    }

    return error instanceof Error && error.message.trim() ? error.message.trim() : DEFAULT_ERROR_MESSAGE;
};


export { DEFAULT_ERROR_MESSAGE, getErrorCode };
export default getErrorMessage;
