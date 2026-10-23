import axios from "axios";


const DEFAULT_ERROR_MESSAGE = "Не удалось выполнить запрос. Попробуйте ещё раз.";


const getErrorMessage = ( error: unknown ): string => {
    if (axios.isAxiosError(error)) {
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


export { DEFAULT_ERROR_MESSAGE };
export default getErrorMessage;
