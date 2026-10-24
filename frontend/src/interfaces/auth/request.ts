interface IPersonalDataConsentAcceptance {
    accepted: true;
    document_version: string;
    document_sha256: string;
}


interface IRegisterRequest {
    email: string;
    password: string;
    display_name: string;
    personal_data_consent: IPersonalDataConsentAcceptance;
}


interface ILoginRequest {
    email: string;
    password: string;
}


interface ITokenRequest {
    token: string;
}


interface IEmailRequest {
    email: string;
}


interface IResetPasswordRequest {
    token: string;
    new_password: string;
}


interface IRegisterParams {
    data: IRegisterRequest;
}


interface ILoginParams {
    data: ILoginRequest;
}


interface IVerifyEmailParams {
    data: ITokenRequest;
}


interface IResendVerificationParams {
    data: IEmailRequest;
}


interface IForgotPasswordParams {
    data: IEmailRequest;
}


interface IResetPasswordParams {
    data: IResetPasswordRequest;
}


export type { IPersonalDataConsentAcceptance, IRegisterRequest, ILoginRequest, ITokenRequest, IEmailRequest, IResetPasswordRequest, IRegisterParams, ILoginParams, IVerifyEmailParams, IResendVerificationParams, IForgotPasswordParams, IResetPasswordParams };
