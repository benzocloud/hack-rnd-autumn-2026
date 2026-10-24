import type { ICompany, ICommitImportRequest, IConnection, ICreateCompanyRequest, ICreateConnectionRequest, ICreateInvitationRequest, ICreateMatchRequest, ICreatePaymentRequest, ICreateTransactionRequest, IImport, IInvitation, IMatch, IPayment, ITransaction, ISetBalanceRequest, IStartSyncRequest, IUpdateCategoryRequest, IUpdateCompanyRequest, IUpdatePaymentRequest } from "@/interfaces";

import { checkVersion, expiresAt, findItem, inPeriod, mockError, mockId, nextVersion, now, paginate, touchData } from "./helpers";
import { createCompanyData, fixture, type IMockCompanyData, type IMockState } from "./state";


const refreshPayment = ( company: IMockCompanyData, payment: IPayment ) => {
    const matched = company.matches.filter(( match ) => match.payment_id === payment.id).reduce(( total, match ) => total + BigInt(match.amount.amount_minor), BigInt(0));
    const remaining = BigInt(payment.amount.amount_minor) - matched;

    payment.remaining_amount.amount_minor = String(remaining > BigInt(0) ? remaining : BigInt(0));
    payment.status = remaining <= BigInt(0) ? "matched" : matched > BigInt(0) ? "partially_matched" : "planned";
    payment.version = nextVersion(payment.version);
    touchData(company);
};


const createImport = ( company: IMockCompanyData, data: unknown, connectionId?: string ) => {
    const record = fixture<Extract<IImport, { status: "preview_ready" }>>("Import");
    const connection = connectionId ? findItem(company.connections, connectionId) : null;
    const form = typeof FormData !== "undefined" && data instanceof FormData ? data : null;

    if (connection?.status === "disabled") {
        throw mockError(409, "Подключение отключено.");
    }

    record.id = mockId("imp");
    record.account_id = company.account.id;
    record.source_company_id = connection?.source_company_id || String(form?.get("source_company_id") || "C001");
    record.created_at = now();
    record.updated_at = now();
    record.version = "1";
    record.preview.account_id = record.account_id;
    record.preview.source_company_id = record.source_company_id;
    record.preview.base_data_version = company.account.data_version;
    record.preview.preview_version = "1";
    record.preview.row_count = 1;
    record.preview.new_transaction_count = 1;
    record.preview.already_present_count = 0;
    record.preview.balance_observations = [];
    record.preview.issues = [];
    record.preview.warning_count = 0;
    record.preview.transaction_source = company.account.transaction_source;
    record.preview.would_bind_transaction_source = null;

    if (connection) {
        record.source_kind = "mock_external";
        record.connection_id = connection.id;
        record.adapter = connection.provider_id;
        record.record_kinds = (data as IStartSyncRequest).record_kinds;
        record.preview.record_kinds = record.record_kinds;
        record.preview.period_start = (data as IStartSyncRequest).period_start;
        record.preview.period_end = (data as IStartSyncRequest).period_end;
        record.preview.new_transaction_count = record.record_kinds.includes("transactions") ? 1 : 0;
        record.preview.new_payment_count = record.record_kinds.includes("planned_payments") ? 1 : 0;
    }

    company.imports.unshift(record);

    return { id: record.id, status: "queued", status_url: `/api/v1/companies/${company.account.company_id}/imports/${record.id}`, poll_after_ms: 300 };
};


const handleMockResources = ( method: string, segments: string[], data: unknown, params: Record<string, unknown>, state: IMockState ): unknown => {
    const [ , companyId, resource, itemId, action, actionId ] = segments;

    if (segments.length === 1) {
        if (method === "GET") {
            return paginate(state.companies, params);
        }

        const request = data as ICreateCompanyRequest;
        const company: ICompany = { ...fixture<ICompany>("Company"), ...request, inn: request.inn ?? null, id: mockId("cmp"), default_account_id: mockId("acc"), version: "1", created_at: now() };

        state.companies.unshift(company);
        state.companyData[company.id] = createCompanyData(company);

        if (state.user) {
            state.companyData[company.id].members.push({ user_id: state.user.id, email: state.user.email, display_name: state.user.display_name, role: "owner", joined_at: now() });
        }

        return company;
    }

    const company = findItem(state.companies, companyId);
    const bucket = state.companyData[companyId];

    if (!resource) {
        if (method === "PATCH") {
            const request = data as IUpdateCompanyRequest;

            checkVersion(company.version, request.version);
            Object.assign(company, request, { version: nextVersion(company.version) });
        }

        return company;
    }

    if (resource === "account") {
        if (method === "PUT") {
            const request = data as ISetBalanceRequest;

            checkVersion(bucket.account.data_version, request.expected_data_version);
            bucket.account.balance = request.balance;
            bucket.account.balance_as_of = request.as_of;
            bucket.account.balance_source = "manual";
            touchData(bucket);
        }

        return bucket.account;
    }

    if (resource === "categories") {
        return paginate(fixture<{ items: unknown[] }>("CategoryList").items, params);
    }

    if (resource === "members") {
        if (method === "DELETE") {
            bucket.members = bucket.members.filter(( member ) => member.user_id !== itemId);

            return undefined;
        }

        return paginate(bucket.members, params);
    }

    if (resource === "invitations") {
        if (method === "DELETE") {
            const invitation = findItem(bucket.invitations, itemId);

            invitation.status = "revoked";
            invitation.version = nextVersion(invitation.version);

            return undefined;
        }

        if (method === "POST") {
            const invitation: IInvitation = { ...fixture<IInvitation>("Invitation"), id: mockId("inv"), company_id: company.id, company_name: company.name, email: (data as ICreateInvitationRequest).email, created_at: now(), expires_at: expiresAt(), version: "1" };

            bucket.invitations.unshift(invitation);

            return invitation;
        }

        return paginate(bucket.invitations, params);
    }

    if (resource === "transactions") {
        if (method === "POST") {
            const request = data as ICreateTransactionRequest;

            checkVersion(bucket.account.data_version, request.expected_data_version);

            const transaction = { ...fixture<ITransaction>("Transaction"), ...request, counterparty_inn: request.counterparty_inn ?? null, category_id: request.category_id ?? null, id: mockId("txn"), account_id: bucket.account.id, source: fixture<IPayment>("Payment").source, created_at: now(), version: "1" };

            delete (transaction as Partial<ICreateTransactionRequest>).expected_data_version;
            bucket.transactions.unshift(transaction);
            touchData(bucket);

            return transaction;
        }

        if (method === "PATCH") {
            const transaction = findItem(bucket.transactions, itemId);
            const request = data as IUpdateCategoryRequest;

            checkVersion(transaction.version, request.version);
            transaction.category_id = request.category_id;
            transaction.version = nextVersion(transaction.version);
            touchData(bucket);

            return transaction;
        }

        return paginate(bucket.transactions.filter(( transaction ) => inPeriod(transaction.operation_date, params) && (!params.direction || transaction.direction === params.direction) && (!params.category_id || transaction.category_id === params.category_id)), params);
    }

    if (resource === "external-providers") {
        return paginate(fixture<{ items: unknown[] }>("ExternalProviderList").items, params);
    }

    if (resource === "connections") {
        if (action === "sync") {
            return createImport(bucket, data, itemId);
        }

        if (method === "DELETE") {
            const connection = findItem(bucket.connections, itemId);

            connection.status = "disabled";
            connection.version = nextVersion(connection.version);

            return undefined;
        }

        if (method === "POST") {
            const connection = { ...fixture<IConnection>("Connection"), ...data as ICreateConnectionRequest, id: mockId("con"), company_id: companyId, account_id: bucket.account.id, version: "1", created_at: now() };

            bucket.connections.unshift(connection);

            return connection;
        }

        return paginate(bucket.connections, params);
    }

    if (resource === "imports") {
        if (method === "POST" && !itemId) {
            return createImport(bucket, data);
        }

        if (!itemId) {
            return paginate(bucket.imports, params);
        }

        const record = findItem(bucket.imports, itemId);

        if (action === "issues") {
            return paginate(record.preview?.issues || [], params);
        }

        if (action === "commit" && record.preview && record.status !== "applied") {
            const request = data as ICommitImportRequest;

            checkVersion(record.version, request.version);
            checkVersion(bucket.account.data_version, request.expected_data_version);
            touchData(bucket);

            const applied: IImport = { ...record, status: "applied", applied_data_version: bucket.account.data_version, preview: record.preview, error: null, version: nextVersion(record.version), updated_at: now() };

            bucket.imports = bucket.imports.map(( item ) => item.id === record.id ? applied : item);

            if (record.preview.new_transaction_count) {
                bucket.transactions.unshift({ ...fixture<ITransaction>("Transaction"), id: mockId("txn"), account_id: bucket.account.id, source: { ...fixture<ITransaction>("Transaction").source, import_id: record.id, connection_id: record.connection_id, kind: record.source_kind }, version: "1", created_at: now() });
            }

            if (record.preview.new_payment_count) {
                bucket.payments.unshift({ ...fixture<IPayment>("Payment"), id: mockId("pay"), account_id: bucket.account.id, source: { ...fixture<IPayment>("Payment").source, import_id: record.id, connection_id: record.connection_id, kind: record.source_kind }, version: "1", created_at: now() });
            }

            return applied;
        }

        return record;
    }

    if (resource === "payments") {
        if (!itemId) {
            if (method === "POST") {
                const request = data as ICreatePaymentRequest;
                const payment: IPayment = { ...fixture<IPayment>("Payment"), ...request, category_id: request.category_id ?? null, id: mockId("pay"), account_id: bucket.account.id, remaining_amount: { ...request.amount }, created_at: now(), version: "1" };

                bucket.payments.unshift(payment);
                touchData(bucket);

                return payment;
            }

            return paginate(bucket.payments.filter(( payment ) => inPeriod(payment.scheduled_date, params) && (!params.status || payment.status === params.status)), params);
        }

        const payment = findItem(bucket.payments, itemId);

        if (action === "matches") {
            if (method === "POST") {
                const request = data as ICreateMatchRequest;
                const transaction = findItem(bucket.transactions, request.transaction_id);

                checkVersion(payment.version, request.payment_version);
                checkVersion(transaction.version, request.transaction_version);

                if (payment.status === "cancelled" || payment.direction !== transaction.direction || BigInt(request.amount.amount_minor) <= BigInt(0) || BigInt(request.amount.amount_minor) > BigInt(payment.remaining_amount.amount_minor)) {
                    throw mockError(409, "Невозможно сопоставить платёж и операцию.");
                }

                const match: IMatch = { ...fixture<IMatch>("Match"), id: mockId("match"), payment_id: payment.id, transaction_id: transaction.id, amount: request.amount, created_at: now() };

                bucket.matches.unshift(match);
                refreshPayment(bucket, payment);

                return match;
            }

            if (method === "DELETE") {
                checkVersion(payment.version, params.payment_version as string | undefined);
                bucket.matches = bucket.matches.filter(( match ) => match.id !== actionId || match.payment_id !== payment.id);
                refreshPayment(bucket, payment);

                return undefined;
            }

            return paginate(bucket.matches.filter(( match ) => match.payment_id === payment.id), params);
        }

        if (method === "PATCH") {
            const request = data as IUpdatePaymentRequest;

            checkVersion(payment.version, request.version);
            Object.assign(payment, request);
            refreshPayment(bucket, payment);
        }

        if (method === "DELETE") {
            checkVersion(payment.version, params.version as string | undefined);
            payment.status = "cancelled";
            payment.version = nextVersion(payment.version);
            touchData(bucket);

            return undefined;
        }

        return payment;
    }

    throw mockError(404, "Мок-ответ для этого запроса не найден.");
};


export default handleMockResources;
