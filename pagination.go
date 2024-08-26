package wsqlx

type PaginationInput struct {
	Page     int64
	PageSize int64
}

type PaginationOutput struct {
	Page      int64
	PageSize  int64
	PageCount int64
	TotalData int64
}

func (p PaginationInput) Offset() int64 {
	offset := int64(0)
	if p.Page > 0 {
		offset = (p.Page - 1) * p.PageSize
	}

	return offset
}

func getPageCount(pageSize, totalData int64) int64 {
	pageCount := int64(1)
	if pageSize > 0 {
		if pageSize >= totalData {
			return pageCount
		}

		if totalData%pageSize == 0 {
			pageCount = totalData / pageSize
		} else {
			pageCount = totalData/pageSize + 1
		}
	}

	return pageCount
}

func CreatePaginationOutput(input PaginationInput, totalData int64) PaginationOutput {
	pageCount := getPageCount(input.PageSize, totalData)
	return PaginationOutput{
		Page:      input.Page,
		PageSize:  input.PageSize,
		TotalData: totalData,
		PageCount: pageCount,
	}
}
