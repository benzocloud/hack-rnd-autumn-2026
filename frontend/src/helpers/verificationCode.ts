const MOCK_CODE_LENGTH = 6;
const mockCodePattern = new RegExp(`^\\d{${MOCK_CODE_LENGTH}}$`);


const isMockVerificationCode = ( value: unknown ): value is string => typeof value === "string" && mockCodePattern.test(value);


export { MOCK_CODE_LENGTH, isMockVerificationCode };
