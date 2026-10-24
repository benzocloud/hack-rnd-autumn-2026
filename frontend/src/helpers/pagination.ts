interface IPage<T> {
    items: T[];
    next_cursor: string | null;
}


const collectPages = async <T,>( loader: ( cursor?: string ) => Promise<IPage<T>> ): Promise<T[]> => {
    const items: T[] = [];
    const cursors = new Set<string>();
    let cursor: string | undefined;

    do {
        const page = await loader(cursor);

        items.push(...page.items);
        cursor = page.next_cursor || undefined;

        if (cursor && cursors.has(cursor)) {
            throw new Error("Не удалось загрузить весь список. Повторите запрос.");
        }

        if (cursor) {
            cursors.add(cursor);
        }
    } while (cursor);

    return items;
};


export { collectPages };
